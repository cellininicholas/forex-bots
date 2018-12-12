# Some bots

## Install

1. Create OANDA practice account
2. Create some v20 sub accounts (one pet intended bot)
3. Find the dev portal on your own and key a API key
4. `cp .example_env .env`
5. add your API key
6. `cp bots_example.json bots.json`
7. design your bots. The account ids are listed on your OANDA account management page
8. `go get github.com/byronhallett/goanda`
9. `go get github.com/joho/godotenv`
10. `go run main.go`

## OANDA short term bot

### JSON bot parameters example

```json
[
  {
    "name": "Slow bot",
    "account": "account_id_1",
    "params": {
      "candleGranularity": "M1",
      "candleCount": "500",
      "momentumPeriod": 300,
      "rsiPeriod": 14,
      "stDevPeriod": 14,
      "volumeFactor": 100,
      "SMAPeriod": 200,
      "takeProfitFactor": 0.00015,
      "stopLossFactor": 0.00200
    }
  },
]
```

### Simple algorithm, so far

- Iterate over pairs without current trade
- Get candles for frequency C
- Compute the momentum over period M
- Compute the RSI over period R
- Compute the SMA over period S
- Compute the stdDev value over period D
- Determine if this pair is due to return to trend-line
- Enter into trade with TP and SL, proportional to stdDev by factor P and L
