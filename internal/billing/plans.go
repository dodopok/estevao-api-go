// Package billing ports the developer portal billing catalogue
// (Billing::PlanCatalog / Billing::Plan) and its configuration.
package billing

import (
	_ "embed"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// plans.yml is a byte-for-byte copy of the Rails config/plans.yml: structure
// only, the price ids are resolved from the environment.
//
//go:embed plans.yml
var plansYAML []byte

const (
	LegacyPlanCode         = "founder"
	defaultTrialPeriodDays = 7
	DefaultDailyLimit      = 1000
	BaseMinuteLimit        = 60
	defaultSupportEmail    = "dev@dodopok.dev"
)

// SupportedIntervals ports PlanCatalog::SUPPORTED_INTERVALS.
var SupportedIntervals = []string{"month", "year"}

// Price is one currency/interval cell.
type Price struct {
	AmountCents      any
	StripePriceIDEnv any
}

// Interval pairs an interval name with its price, in file order.
type Interval struct {
	Name  string
	Price map[string]any
}

// Currency pairs a currency code with its intervals, in file order.
type Currency struct {
	Code      string
	Intervals []Interval
}

// Plan ports Billing::Plan.
type Plan struct {
	Code        string
	Multiplier  int
	MaxKeys     int
	Purchasable bool
	Prices      []Currency
}

type catalog struct {
	trialPeriodDays int
	plans           []*Plan
}

var (
	once    sync.Once
	loaded  *catalog
	loadErr error
)

func load() (*catalog, error) {
	once.Do(func() {
		var doc yaml.Node
		if err := yaml.Unmarshal(plansYAML, &doc); err != nil {
			loadErr = err
			return
		}
		c := &catalog{trialPeriodDays: defaultTrialPeriodDays}
		if len(doc.Content) == 0 {
			loaded = c
			return
		}
		root := doc.Content[0]
		if v := mapValue(root, "trial_period_days"); v != nil && v.Tag != "!!null" {
			c.trialPeriodDays = int(rb.StringToI(v.Value))
		}
		if list := mapValue(root, "plans"); list != nil {
			for _, item := range list.Content {
				c.plans = append(c.plans, buildPlan(item))
			}
		}
		loaded = c
	})
	return loaded, loadErr
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalarToI(n *yaml.Node) int {
	if n == nil {
		return 0
	}
	return int(rb.StringToI(n.Value))
}

func buildPlan(n *yaml.Node) *Plan {
	p := &Plan{Purchasable: true}
	if v := mapValue(n, "code"); v != nil {
		p.Code = v.Value
	}
	p.Multiplier = scalarToI(mapValue(n, "multiplier"))
	p.MaxKeys = scalarToI(mapValue(n, "max_keys"))
	if v := mapValue(n, "purchasable"); v != nil {
		p.Purchasable = !(v.Tag == "!!bool" && v.Value == "false") && v.Tag != "!!null"
	}
	if prices := mapValue(n, "prices"); prices != nil && prices.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(prices.Content); i += 2 {
			cur := Currency{Code: prices.Content[i].Value}
			ivs := prices.Content[i+1]
			for j := 0; ivs.Kind == yaml.MappingNode && j+1 < len(ivs.Content); j += 2 {
				price := map[string]any{}
				cell := ivs.Content[j+1]
				for k := 0; cell.Kind == yaml.MappingNode && k+1 < len(cell.Content); k += 2 {
					price[cell.Content[k].Value] = cell.Content[k+1].Value
				}
				cur.Intervals = append(cur.Intervals, Interval{Name: ivs.Content[j].Value, Price: price})
			}
			p.Prices = append(p.Prices, cur)
		}
	}
	return p
}

// All ports PlanCatalog.all.
func All() []*Plan {
	c, err := load()
	if err != nil {
		panic(err)
	}
	return c.plans
}

// Purchasable ports PlanCatalog.purchasable.
func Purchasable() []*Plan {
	var out []*Plan
	for _, p := range All() {
		if p.Purchasable {
			out = append(out, p)
		}
	}
	return out
}

// Find ports PlanCatalog.find.
func Find(code string) *Plan {
	for _, p := range All() {
		if p.Code == code {
			return p
		}
	}
	return nil
}

// TrialPeriodDays ports PlanCatalog.trial_period_days.
func TrialPeriodDays() int {
	c, err := load()
	if err != nil {
		panic(err)
	}
	return c.trialPeriodDays
}

// SupportedCurrencies ports PlanCatalog.supported_currencies.
func SupportedCurrencies() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range Purchasable() {
		for _, c := range p.Prices {
			if !seen[c.Code] {
				seen[c.Code] = true
				out = append(out, c.Code)
			}
		}
	}
	return out
}

// BaseDailyLimit ports Billing::Plan.base_daily_limit.
func BaseDailyLimit() int { return config.PositiveInt("API_KEY_DAILY_LIMIT", DefaultDailyLimit) }

// DailyRequestLimit ports Plan#daily_request_limit.
func (p *Plan) DailyRequestLimit() int { return BaseDailyLimit() * p.Multiplier }

// MinuteRequestLimit ports Plan#minute_request_limit.
func (p *Plan) MinuteRequestLimit() int { return BaseMinuteLimit * p.Multiplier }

// PriceFor ports Plan#price_for.
func (p *Plan) PriceFor(currency, interval string) map[string]any {
	for _, c := range p.Prices {
		if c.Code != currency {
			continue
		}
		for _, iv := range c.Intervals {
			if iv.Name == interval {
				return iv.Price
			}
		}
	}
	return nil
}

// StripePriceID ports Plan#stripe_price_id.
func (p *Plan) StripePriceID(currency, interval string) string {
	price := p.PriceFor(currency, interval)
	if len(price) == 0 {
		return ""
	}
	v := os.Getenv(rb.ToS(price["stripe_price_id_env"]))
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return v
}

// AsJSON ports Plan#as_json.
func (p *Plan) AsJSON() *rb.Map {
	prices := rb.NewMap()
	for _, c := range p.Prices {
		ivs := rb.NewMap()
		for _, iv := range c.Intervals {
			ivs.Set(iv.Name, rb.M("amount_cents", rb.StringToI(rb.ToS(iv.Price["amount_cents"]))))
		}
		prices.Set(c.Code, ivs)
	}
	return rb.M("code", p.Code, "purchasable", p.Purchasable, "multiplier", p.Multiplier, "max_keys", p.MaxKeys,
		"daily_request_limit", p.DailyRequestLimit(), "minute_request_limit", p.MinuteRequestLimit(), "prices", prices)
}

// SupportEmail ports Billing.support_email.
func SupportEmail() string {
	if v, ok := os.LookupEnv("BILLING_SUPPORT_EMAIL"); ok {
		return v
	}
	return defaultSupportEmail
}
