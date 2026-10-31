package screener

// Row is one security as the screener sees it.
//
// Money and percentage fields are pointers because a metric can genuinely be
// absent: a security with one stored session has no 20-day average, and a zero
// there would be a claim about the market rather than a gap in the data. Alert
// evaluation reads values through Value so the metric registry stays the single
// definition of what a name means.
type Row struct {
	SecurityID    string  `json:"security_id"`
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Sector        string  `json:"sector"`
	SectorCode    string  `json:"sector_code"`
	Instrument    string  `json:"instrument"`
	Status        string  `json:"status"`
	TradingDate   string  `json:"trading_date"`
	Close         *string `json:"close,omitempty"`
	PreviousClose *string `json:"previous_close,omitempty"`
	ChangePct     *string `json:"change_pct,omitempty"`
	Volume        *string `json:"volume,omitempty"`
	Turnover      *string `json:"turnover,omitempty"`
	Transactions  *int64  `json:"transactions,omitempty"`
	MarketCap     *string `json:"market_cap,omitempty"`
	FloatCap      *string `json:"float_market_cap,omitempty"`
	FreeFloatPct  *string `json:"free_float_pct,omitempty"`
	ListedShares  *string `json:"listed_shares,omitempty"`
	PublicShares  *string `json:"public_shares,omitempty"`
	SMA20         *string `json:"sma_20,omitempty"`
	SMA50         *string `json:"sma_50,omitempty"`
	High52W       *string `json:"high_52w,omitempty"`
	Low52W        *string `json:"low_52w,omitempty"`
	PctFromHigh   *string `json:"pct_from_52w_high,omitempty"`
	VolumeRatio   *string `json:"volume_ratio_20,omitempty"`
	Return5D      *string `json:"return_5d,omitempty"`
	Return20D     *string `json:"return_20d,omitempty"`
	Bars          int     `json:"bars"`
}

// Value returns a metric's value for this row, and whether it is present.
//
// An absent value is not zero: a security with no stored session has no price,
// and an alert on "close below 400" must not fire because the price is missing.
func (r Row) Value(metric string) (string, bool) {
	pick := func(p *string) (string, bool) {
		if p == nil || *p == "" {
			return "", false
		}
		return *p, true
	}
	switch metric {
	case "close":
		return pick(r.Close)
	case "previous_close":
		return pick(r.PreviousClose)
	case "change_pct":
		return pick(r.ChangePct)
	case "volume":
		return pick(r.Volume)
	case "turnover":
		return pick(r.Turnover)
	case "transactions":
		if r.Transactions == nil {
			return "", false
		}
		return itoa(*r.Transactions), true
	case "market_cap":
		return pick(r.MarketCap)
	case "float_market_cap":
		return pick(r.FloatCap)
	case "free_float_pct":
		return pick(r.FreeFloatPct)
	case "listed_shares":
		return pick(r.ListedShares)
	case "public_shares":
		return pick(r.PublicShares)
	case "sma_20":
		return pick(r.SMA20)
	case "sma_50":
		return pick(r.SMA50)
	case "return_5d":
		return pick(r.Return5D)
	case "return_20d":
		return pick(r.Return20D)
	case "high_52w":
		return pick(r.High52W)
	case "low_52w":
		return pick(r.Low52W)
	case "pct_from_52w_high":
		return pick(r.PctFromHigh)
	case "volume_ratio_20":
		return pick(r.VolumeRatio)
	case "bars":
		if r.Bars <= 0 {
			return "", false
		}
		return itoa(int64(r.Bars)), true
	}
	return "", false
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
