// Matching scanned hardware values against GLPI entries that were typed in
// by hand over the years, with inconsistent spelling: "Core i5-7200U" next
// to "Intel Core i5 14600K", "Lenovo ThinkPad T470" next to "Thinkpad T470",
// "DIMM 8 GB DDR3 1600 MHz" next to "DIMM DDR4 4GB 2133MHz". Both sides are
// reduced to a set of lowercase words and compared, instead of requiring
// the scan to reproduce one exact spelling.
package glpi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	trademarkRe = regexp.MustCompile(`\((r|tm|c)\)|[®™©]`)
	clockRe     = regexp.MustCompile(`@.*$`)
	separatorRe = regexp.MustCompile(`[^\p{L}\p{N}.]+`)
	numberRe    = regexp.MustCompile(`^\d+(\.\d+)?$`)
	ordinalRe   = regexp.MustCompile(`^\d+(st|nd|rd|th)$`)
	ghzRe       = regexp.MustCompile(`^\d+(\.\d+)?ghz$`)
	unitRe      = regexp.MustCompile(`^\d+(\.\d+)?(mb|gb|tb|mhz|ghz)$`)
)

// nameAliases rewrites spellings that differ in more than case or
// punctuation. Applied to the space-padded, space-joined word list.
var nameAliases = strings.NewReplacer(
	" hewlett packard ", " hp ",
	" so dimm ", " sodimm ",
	" asustek ", " asus ",
	" micro star international ", " msi ",
)

var unitWords = map[string]bool{"mb": true, "gb": true, "tb": true, "mhz": true, "ghz": true}

// noiseWords carry no identity: "Intel(R) Core(TM) i5-7200U CPU", "Dell
// Inc.", "11th Gen Intel ...". Dropped on both sides.
var noiseWords = map[string]bool{
	"cpu": true, "processor": true, "gen": true,
	"inc": true, "corp": true, "corporation": true, "co": true, "ltd": true, "limited": true, "gmbh": true,
}

func canonicalTokens(s string) []string {
	s = strings.ToLower(s)
	s = trademarkRe.ReplaceAllString(s, " ")
	s = clockRe.ReplaceAllString(s, "")

	var parts []string
	for _, p := range separatorRe.Split(s, -1) {
		if p = strings.Trim(p, "."); p != "" {
			parts = append(parts, p)
		}
	}
	parts = strings.Fields(nameAliases.Replace(" " + strings.Join(parts, " ") + " "))

	var tokens []string
	for i := 0; i < len(parts); i++ {
		t := parts[i]
		// "8 GB" and "8GB" are the same word.
		if numberRe.MatchString(t) && i+1 < len(parts) && unitWords[parts[i+1]] {
			t += parts[i+1]
			i++
		}
		if noiseWords[t] || ordinalRe.MatchString(t) || ghzRe.MatchString(t) {
			continue
		}
		tokens = append(tokens, t)
	}
	return tokens
}

func hasDigit(s string) bool {
	return strings.IndexFunc(s, unicode.IsDigit) >= 0
}

func toSet(tokens []string) map[string]bool {
	set := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		set[t] = true
	}
	return set
}

// matchQuality reports whether cand names the same thing as scan, and how
// many scanned words it leaves unexplained (fewer is closer). Every word of
// the GLPI entry must appear in the scan, and every word containing a digit
// — model numbers, capacities, versions — must appear on both sides, so
// "Windows 11" matches "Microsoft Windows 11 Pro" but "Core i5-7300U" never
// matches "Core i5-7200U".
func matchQuality(scan, cand map[string]bool) (ok bool, extra int) {
	if len(cand) == 0 {
		return false, 0
	}
	for t := range cand {
		if !scan[t] {
			return false, 0
		}
	}
	for t := range scan {
		if hasDigit(t) && !cand[t] {
			return false, 0
		}
	}
	return true, len(scan) - len(cand)
}

// bestMatch picks the entry closest to value: fewest unexplained words,
// then the lowest (oldest) id among exact duplicates. hints are words the
// scan implies without spelling out — e.g. the manufacturer for a model
// named "Latitude 7490" — so "Dell Latitude 7490" wins over a bare
// "Latitude 7490" entry. Returns 0 when nothing matches.
func bestMatch(value string, hints []string, entries []namedEntry, field string) int {
	valueTokens := canonicalTokens(value)
	if len(valueTokens) == 0 {
		return 0
	}
	scan := toSet(valueTokens)
	for _, h := range hints {
		for _, t := range canonicalTokens(h) {
			if !hasDigit(t) {
				scan[t] = true
			}
		}
	}

	bestID, bestExtra := 0, -1
	for _, e := range entries {
		ok, extra := matchQuality(scan, toSet(canonicalTokens(e.label(field))))
		if !ok {
			continue
		}
		if bestExtra == -1 || extra < bestExtra || (extra == bestExtra && e.ID < bestID) {
			bestID, bestExtra = e.ID, extra
		}
	}
	return bestID
}

// searchToken picks the scanned word most likely to narrow GLPI's
// server-side substring search: the longest one containing a digit, as long
// as it appears verbatim in the raw text (joined units like "8gb" may be
// stored as "8 GB"). Empty means fetch the whole list — fine for the small
// dropdowns (manufacturers, types) that have no model numbers.
func searchToken(tokens []string) string {
	best := ""
	for _, t := range tokens {
		if hasDigit(t) && !unitRe.MatchString(t) && len(t) > len(best) {
			best = t
		}
	}
	return best
}

type namedEntry struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Designation string `json:"designation"`
}

func (e namedEntry) label(field string) string {
	if field == "designation" {
		return e.Designation
	}
	return e.Name
}

// ListCatalogNames returns every distinct value currently in one of GLPI's
// hardware catalogs (Manufacturer/ComputerModel/ComputerType/
// OperatingSystem/OperatingSystemVersion: field "name"; DeviceProcessor/
// DeviceGraphicCard: field "designation") — read-only, powers the New
// Device form's type-to-filter suggestions so a technician sees what
// already exists instead of guessing at GLPI's exact spelling.
func (c *Client) ListCatalogNames(ctx context.Context, sess *Session, itemtype, field string) ([]string, error) {
	entries, err := c.listEntries(ctx, sess, itemtype, field, "")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(entries))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		v := e.label(field)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		names = append(names, v)
	}
	sort.Strings(names)
	return names, nil
}

// ResolveByNameContains finds the first itemtype entry (e.g. "Group",
// "Location") whose name contains needle (case-insensitive) — for a fixed
// value this app itself specifies (a group/location the technician told
// us by a casual name, not necessarily GLPI's exact stored spelling),
// unlike bestMatch's tolerant multi-token matching built for messy scanned
// hardware strings. Returns the matched id and its real GLPI name (so a
// caller can log/confirm which one got picked), or an error — never 0
// silently — when nothing matches, since a renamed/misspelled fixed value
// should fail loudly, not quietly write nothing.
func (c *Client) ResolveByNameContains(ctx context.Context, sess *Session, itemtype, needle string) (id int, name string, err error) {
	entries, err := c.listEntries(ctx, sess, itemtype, "name", "")
	if err != nil {
		return 0, "", err
	}
	n := strings.ToLower(strings.TrimSpace(needle))
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), n) {
			return e.ID, e.Name, nil
		}
	}
	return 0, "", fmt.Errorf("kein %s-Eintrag enthält %q (%d durchsucht)", itemtype, needle, len(entries))
}

func (c *Client) findBestMatch(ctx context.Context, sess *Session, itemtype, field, value string, hints []string) (int, error) {
	tokens := canonicalTokens(value)
	if len(tokens) == 0 {
		return 0, nil
	}
	entries, err := c.listEntries(ctx, sess, itemtype, field, searchToken(tokens))
	if err != nil {
		return 0, err
	}
	return bestMatch(value, hints, entries, field), nil
}

// listEntries pages through a v1 itemtype, optionally narrowed by GLPI's
// substring search on field.
func (c *Client) listEntries(ctx context.Context, sess *Session, itemtype, field, search string) ([]namedEntry, error) {
	const page = 500
	var all []namedEntry
	for start := 0; ; start += page {
		q := url.Values{"range": {fmt.Sprintf("%d-%d", start, start+page-1)}}
		if search != "" {
			q.Set("searchText["+field+"]", search)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/api.php/v1/%s?%s", c.BaseURL, itemtype, q.Encode()), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("App-Token", c.V1AppToken)
		req.Header.Set("Session-Token", sess.V1SessionToken)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}

		var entries []namedEntry
		if err := json.Unmarshal(body, &entries); err != nil {
			return nil, fmt.Errorf("unexpected response: %s", string(body))
		}
		all = append(all, entries...)

		total := contentRangeTotal(resp.Header.Get("Content-Range"))
		if len(entries) < page || (total > 0 && start+page >= total) {
			return all, nil
		}
	}
}

// contentRangeTotal parses the total out of "0-499/2082"; 0 if absent.
func contentRangeTotal(h string) int {
	i := strings.LastIndex(h, "/")
	if i == -1 {
		return 0
	}
	n, _ := strconv.Atoi(h[i+1:])
	return n
}

// createEntry creates a dropdown ("name") or catalog ("designation") entry
// via v1 and returns its id.
func (c *Client) createEntry(ctx context.Context, sess *Session, itemtype, field, value string) (int, error) {
	body, err := json.Marshal(map[string]any{"input": map[string]any{field: value}})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api.php/v1/%s", c.BaseURL, itemtype), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("App-Token", c.V1AppToken)
	req.Header.Set("Session-Token", sess.V1SessionToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var created v1CreateResponse
	if err := json.Unmarshal(respBody, &created); err != nil {
		return 0, fmt.Errorf("unexpected response: %s", string(respBody))
	}
	return created.ID, nil
}
