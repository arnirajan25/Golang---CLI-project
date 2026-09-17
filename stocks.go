// Command nasdaq-top5 fetches and prints the top 5 most actively traded
// NASDAQ stocks right now, using Yahoo Finance's public "most actives"
// screener endpoint (no API key required).
//
// Usage:
//
//	go run main.go
//	go run main.go -n 10          // show top 10 instead of top 5
//
// Note: this hits a live network endpoint. It needs a normal internet
// connection to run — it will not work in a sandboxed/offline environment.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// screenerResponse mirrors the small slice of Yahoo Finance's JSON response
// that we actually need. The real payload has many more fields; here's only
// declared the ones we only used.
type screenerResponse struct {
	Finance struct {
		Result []struct {
			Quotes []quote `json:"quotes"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"finance"`
}

type quote struct {
	Symbol                 string  `json:"symbol"`
	ShortName              string  `json:"shortName"`
	FullExchangeName       string  `json:"fullExchangeName"`
	RegularMarketPrice     float64 `json:"regularMarketPrice"`
	RegularMarketChangePct float64 `json:"regularMarketChangePercent"`
	RegularMarketVolume    int64   `json:"regularMarketVolume"`
}

const screenerURL = "https://query1.finance.yahoo.com/v1/finance/screener/predefined/saved" +
	"?count=50&scrIds=most_actives&lang=en-US&region=US"

func main() {
	topN := flag.Int("n", 5, "number of stocks to display")
	flag.Parse()

	quotes, err := fetchMostActive()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	nasdaqQuotes := filterNasdaq(quotes)
	if len(nasdaqQuotes) == 0 {
		fmt.Fprintln(os.Stderr, "no NASDAQ-listed stocks found in the current most-active list")
		os.Exit(1)
	}

	// Sort by traded volume, descending — a reasonable proxy for
	// "top stocks trading currently."
	sort.Slice(nasdaqQuotes, func(i, j int) bool {
		return nasdaqQuotes[i].RegularMarketVolume > nasdaqQuotes[j].RegularMarketVolume
	})

	if *topN > len(nasdaqQuotes) {
		*topN = len(nasdaqQuotes)
	}

	printTable(nasdaqQuotes[:*topN])
}

// fetchMostActive calls the Yahoo Finance screener endpoint and returns
// the raw list of quotes (not yet filtered to NASDAQ).
func fetchMostActive() ([]quote, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest(http.MethodGet, screenerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	// Yahoo's endpoint blocks requests with no browser-like User-Agent.
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; nasdaq-top5-cli/1.0; +https://example.com)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Yahoo Finance: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed screenerResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	if len(parsed.Finance.Result) == 0 {
		return nil, fmt.Errorf("empty result set (response: %s)", truncate(string(body), 200))
	}

	return parsed.Finance.Result[0].Quotes, nil
}

// filterNasdaq keeps only quotes whose exchange name indicates NASDAQ.
// Yahoo reports several NASDAQ tiers, e.g. "NasdaqGS", "NasdaqGM", "NasdaqCM".
func filterNasdaq(quotes []quote) []quote {
	var out []quote
	for _, q := range quotes {
		if strings.Contains(strings.ToLower(q.FullExchangeName), "nasdaq") {
			out = append(out, q)
		}
	}
	return out
}

func printTable(quotes []quote) {
	fmt.Printf("%-4s  %-8s  %-28s  %12s  %10s  %14s\n",
		"#", "Symbol", "Name", "Price (USD)", "Change %", "Volume")
	fmt.Println(strings.Repeat("-", 84))

	for i, q := range quotes {
		fmt.Printf("%-4d  %-8s  %-28s  %12.2f  %9.2f%%  %14s\n",
			i+1,
			q.Symbol,
			truncate(q.ShortName, 28),
			q.RegularMarketPrice,
			q.RegularMarketChangePct,
			formatVolume(q.RegularMarketVolume),
		)
	}
}

func formatVolume(v int64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(v)/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(v)/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fK", float64(v)/1_000)
	default:
		return fmt.Sprintf("%d", v)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
