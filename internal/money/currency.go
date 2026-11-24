package money

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Minor units from the 2026-09-17 ISO 4217 list maintained by SIX, retaining
// the app's established zero-decimal display for IDR and historical BYR.
// https://www.six-group.com/dam/download/financial-information/data-center/iso-currrency/lists/list-one.xml
//
//go:embed currency-exponents.json
var currencyExponentJSON []byte

var currencyExponents = loadCurrencyExponents()

func loadCurrencyExponents() map[string]int {
	var exponents map[string]int
	if err := json.Unmarshal(currencyExponentJSON, &exponents); err != nil {
		panic(err)
	}
	return exponents
}

func CurrencyExponent(currency string) int {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if exponent, ok := currencyExponents[code]; ok {
		return exponent
	}
	return currencyExponents["DEFAULT"]
}
