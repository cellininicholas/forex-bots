package main

import (
	"fmt"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"

	"github.com/AwolDes/goanda"
	"github.com/joho/godotenv"
)

const (
	market        string  = "MARKET"
	fok           string  = "FOK"
	volumePercent float64 = 100
	orderFill     string  = "DEFAULT"
)

func startOandaConnection() (*goanda.OandaConnection, string) {
	// Load the env
	if godotenv.Load() != nil { // err
		log.Fatal("No api keys")
	}
	key := os.Getenv("OANDA_API_KEY")
	accountID := os.Getenv("OANDA_ACCOUNT_ID")
	oanda := goanda.NewConnection(accountID, key, false)
	return oanda, accountID
}

func getCurrencies(connection *goanda.OandaConnection, accountID string) *goanda.AccountInstruments {
	ret := goanda.AccountInstruments{}
	instruments := connection.GetAccountInstruments(accountID)
	for _, inst := range instruments.Instruments {
		if inst.Type == "CURRENCY" {
			ret.Instruments = append(ret.Instruments, inst)
		}
	}
	return &ret
}

func getInstrumentsWithoutPositions(connection *goanda.OandaConnection, fromInstruments *goanda.AccountInstruments) *goanda.AccountInstruments {
	ret := goanda.AccountInstruments{}
	// O(n) to store the open trades in a map
	keys := map[string]bool{}
	for _, t := range connection.GetOpenTrades().Trades {
		keys[t.Instrument] = true
	}
	// O(n) to check which instruments are open
	for _, instrument := range fromInstruments.Instruments {
		if !keys[instrument.Name] {
			ret.Instruments = append(ret.Instruments, instrument)
		}
	}
	return &ret
}

func runBot(connection *goanda.OandaConnection, instruments *goanda.AccountInstruments, momentumPeriod int, rsiPeriod int, takeProfitFactor float64, stopLossFactor float64) {
	// candleFrequency := "1M" // TODO, parametrise
	noPos := getInstrumentsWithoutPositions(connection, instruments)
	for _, instrument := range noPos.Instruments {
		// Assess this one
		prices := connection.GetInstrumentPrice(instrument.Name).Prices
		lastPrice := prices[len(prices)-1].CloseoutBid
		// We found a good'n
		response := connection.CreateOrder(goanda.OrderPayload{Order: goanda.OrderBody{
			Instrument:   instrument.Name,
			Units:        int(lastPrice * volumePercent),
			Type:         market,
			TimeInForce:  fok,
			PositionFill: orderFill,
		}})
		spew.Dump(response)
		break
	}
}

func main() {
	fmt.Println("Connecting")
	connection, accountID := startOandaConnection()
	fmt.Println("getting currencies")
	currencies := getCurrencies(connection, accountID)
	fmt.Println("Filtering currencies")
	runBot(connection, currencies, 450, 14, 0.001, 0.001)
}
