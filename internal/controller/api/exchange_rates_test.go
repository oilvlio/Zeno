package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testGoogleFinanceRates = map[string]string{
	"USD": "8",
	"HKD": "0.92",
	"EUR": "7.2",
	"GBP": "9",
	"JPY": "0.05",
	"SGD": "5.3",
	"AUD": "4.7",
	"CAD": "4.8",
	"KRW": "0.0046",
}

func newGoogleFinanceTestServer(t *testing.T, missingCurrency string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "Mozilla/5.0") {
			http.Error(w, "browser user agent required", http.StatusBadRequest)
			return
		}
		currency := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/quote/"), "-CNY")
		rate, ok := testGoogleFinanceRates[currency]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if currency == missingCurrency {
			_, _ = w.Write([]byte(`<div class="N6SYTe"><span><span>unavailable</span></span></div>`))
			return
		}
		_, _ = fmt.Fprintf(w, `<div class="gO24Ff">%s / CNY</div><div class="N6SYTe"><span><span>%s</span></span></div>`, currency, rate)
	}))
}

func googleFinanceTestEndpoint(server *httptest.Server) string {
	return server.URL + "/quote/{currency}-CNY?hl=en"
}

func TestExchangeRatesConvertRenewalCostToMonthlyCNY(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	currencyAPI := newGoogleFinanceTestServer(t, "")
	defer currencyAPI.Close()

	if err := store.RefreshExchangeRates(context.Background(), currencyAPI.Client(), googleFinanceTestEndpoint(currencyAPI)); err != nil {
		t.Fatalf("refresh exchange rates: %v", err)
	}
	created, err := store.CreateAdminNode(context.Background(), AdminNodeCreateRequest{
		DisplayName:     "USD yearly node",
		BillingCycle:    "年",
		RenewalAmount:   adminOptionalFloat{Set: true, Valid: true, Value: 24},
		RenewalCurrency: "USD",
	})
	if err != nil {
		t.Fatalf("create admin node: %v", err)
	}
	if created.RenewalAmount == nil || *created.RenewalAmount != 24 || created.RenewalCurrency != "USD" {
		t.Fatalf("created renewal fields = amount=%v currency=%q", created.RenewalAmount, created.RenewalCurrency)
	}

	summary, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summary.Nodes) != 1 {
		t.Fatalf("summary nodes = %d, want 1", len(summary.Nodes))
	}
	node := summary.Nodes[0]
	if node.RenewalAmount == nil || *node.RenewalAmount != 24 || node.RenewalCurrency != "USD" || node.BillingCycle != "年" {
		t.Fatalf("public renewal fields = %+v", node)
	}
	if node.MonthlyCostCNY == nil || *node.MonthlyCostCNY != 16 {
		t.Fatalf("monthly cost CNY = %v, want 16", node.MonthlyCostCNY)
	}
	if len(summary.ExchangeRates) != len(exchangeRateCurrencies) || summary.ExchangeRates["CNY"] != 1 || summary.ExchangeRates["USD"] != 8 {
		t.Fatalf("public exchange rates = %#v", summary.ExchangeRates)
	}

	var sourceDate string
	if err := store.db.QueryRow(`SELECT source_date FROM exchange_rates WHERE currency = 'USD'`).Scan(&sourceDate); err != nil {
		t.Fatalf("read cached USD source date: %v", err)
	}
	if want := time.Now().In(time.Local).Format("2006-01-02"); sourceDate != want {
		t.Fatalf("cached USD source date = %q, want current local date %q", sourceDate, want)
	}
}

func TestExchangeRateRefreshRejectsIncompletePayloadWithoutReplacingCache(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "zeno.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	validAPI := newGoogleFinanceTestServer(t, "")
	if err := store.RefreshExchangeRates(context.Background(), validAPI.Client(), googleFinanceTestEndpoint(validAPI)); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	validAPI.Close()

	var initialRate float64
	var initialSourceDate string
	if err := store.db.QueryRow(`SELECT cny_rate, source_date FROM exchange_rates WHERE currency = 'USD'`).Scan(&initialRate, &initialSourceDate); err != nil {
		t.Fatalf("read initial cached USD rate: %v", err)
	}

	invalidAPI := newGoogleFinanceTestServer(t, "USD")
	defer invalidAPI.Close()
	if err := store.RefreshExchangeRates(context.Background(), invalidAPI.Client(), googleFinanceTestEndpoint(invalidAPI)); err == nil {
		t.Fatal("incomplete refresh succeeded, want error")
	}

	var rate float64
	var sourceDate string
	if err := store.db.QueryRow(`SELECT cny_rate, source_date FROM exchange_rates WHERE currency = 'USD'`).Scan(&rate, &sourceDate); err != nil {
		t.Fatalf("read cached USD rate: %v", err)
	}
	if rate != initialRate || sourceDate != initialSourceDate {
		t.Fatalf("cached USD rate/date = %v/%q, want unchanged %v/%q", rate, sourceDate, initialRate, initialSourceDate)
	}
}

func TestParseGoogleFinanceRateSupportsCurrentAndFallbackMarkup(t *testing.T) {
	tests := []struct {
		name string
		html string
		want float64
	}{
		{name: "current", html: `<div class="N6SYTe"><span class=""><span>6.7661</span></span></div>`, want: 6.7661},
		{name: "legacy", html: `<div class="YMlKec fxKbKc">1,234.56</div>`, want: 1234.56},
		{name: "data attribute", html: `<div data-last-price="0.0046"></div>`, want: 0.0046},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseGoogleFinanceRate([]byte(test.html))
			if err != nil {
				t.Fatalf("parse Google Finance rate: %v", err)
			}
			if got != test.want {
				t.Fatalf("rate = %v, want %v", got, test.want)
			}
		})
	}
}
