package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/byronhallett/goanda"
	"github.com/joho/godotenv"
)

// Some constants for OANDA orders
const (
	market               string = "MARKET"
	fireOrKill           string = "FOK"
	immediateOrCancelled string = "IOC"
	defaultFill          string = "DEFAULT"
)

type botParams struct {
	candleGranularity string
	candleCount       string
	momentumPeriod    int
	rsiPeriod         int
	stDevPeriod       int
	volumeFactor      float64
	takeProfitFactor  float64
	stopLossFactor    float64
}

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

func average(candles *[]goanda.Candle) (result float64) {
	for _, c := range *candles {
		result += c.Mid.Close
	}
	result /= math.Max(float64(len(*candles)), 1)
	return
}

func computeMomentum(candles *goanda.BidAskCandles, period int) float64 {
	endIndex := len(candles.Candles) - 1
	spew.Dump(period, endIndex)
	firstHalf := candles.Candles[endIndex-2*period : endIndex-period]
	lastHalf := candles.Candles[endIndex-period : endIndex]
	return average(&lastHalf) - average(&firstHalf)
}

func computeRSI(candles *goanda.BidAskCandles, period int) float64 {
	return 1
}

func computeStandardDeviation(candles *goanda.BidAskCandles, period int) float64 {
	return 2.0
}

func goLong(connection *goanda.OandaConnection, instrument *goanda.Instrument, lastCandle *goanda.Candle, params *botParams) {
	tradeVol := int((1 / lastCandle.Bid.Close) * params.volumeFactor)
	if tradeVol < 0 {
		return
	}
	tp := fmt.Sprintf("%f.6", lastCandle.Ask.Close)
	sl := fmt.Sprintf("%f.6", lastCandle.Bid.Close)
	response := connection.CreateOrder(goanda.OrderPayload{Order: goanda.OrderBody{
		Instrument:       instrument.Name,
		Type:             market,
		Units:            tradeVol,
		TimeInForce:      fireOrKill,
		PositionFill:     defaultFill,
		TakeProfitOnFill: &goanda.OnFill{Price: tp},
		StopLossOnFill:   &goanda.OnFill{Price: sl},
	}})
	fmt.Println("Placed long order:")
	spew.Dump(response.OrderFillTransaction)
}

func goShort(connection *goanda.OandaConnection, instrument *goanda.Instrument, lastCandle *goanda.Candle, params *botParams) {
	tradeVol := int((1 / lastCandle.Ask.Close) * params.volumeFactor)
	if tradeVol < 0 {
		return
	}
	tp := fmt.Sprintf("%f.6", lastCandle.Bid)
	sl := fmt.Sprintf("%f.6", lastCandle.Ask)
	response := connection.CreateOrder(goanda.OrderPayload{Order: goanda.OrderBody{
		Instrument:       instrument.Name,
		Type:             market,
		Units:            -tradeVol,
		TimeInForce:      fireOrKill,
		PositionFill:     defaultFill,
		TakeProfitOnFill: &goanda.OnFill{Price: tp},
		StopLossOnFill:   &goanda.OnFill{Price: sl},
	}})
	fmt.Println("Placed short order:")
	spew.Dump(response.OrderFillTransaction)
}

func analyseAndTrade(connection *goanda.OandaConnection, instrument *goanda.Instrument, params *botParams, wg *sync.WaitGroup) {
	defer wg.Done()
	candles := connection.GetBidAskCandles(instrument.Name, params.candleCount, params.candleGranularity)
	// Assess the candles for Momentum, RSI, ST.DEV
	momentum := computeMomentum(&candles, params.momentumPeriod)
	spew.Dump(momentum)
	return
	isLong := momentum > 0
	rsi := computeRSI(&candles, params.rsiPeriod)
	stDev := computeStandardDeviation(&candles, params.stDevPeriod)
	score := momentum * rsi * stDev

	// If score is high, enter following block
	if score > 1 {
		lastCandle := candles.Candles[len(candles.Candles)-1]
		if isLong {
			goLong(connection, instrument, &lastCandle, params)
		} else {
			goShort(connection, instrument, &lastCandle, params)
		}
	}
}

// runBot can be run on its own thread to allow testing bots simultaneously
func runBot(connection *goanda.OandaConnection, instruments *goanda.AccountInstruments, params *botParams) {
	noPos := getInstrumentsWithoutPositions(connection, instruments)
	var wg sync.WaitGroup
	wg.Add(len(noPos.Instruments))
	for _, instrument := range noPos.Instruments {
		// Get the candles of interest
		go analyseAndTrade(connection, &instrument, params, &wg)
		break
	}
	wg.Wait()
	time.Sleep(time.Second)
}

func main() {
	fmt.Println("Connecting")
	connection, accountID := startOandaConnection()
	fmt.Println("getting currencies")
	currencies := getCurrencies(connection, accountID)
	fmt.Println("Runnning bot")
	runBot(connection, currencies, &botParams{
		candleGranularity: "M1",
		candleCount:       "500",
		momentumPeriod:    450,
		rsiPeriod:         14,
		stDevPeriod:       14,
		volumeFactor:      100,
		takeProfitFactor:  0.001,
		stopLossFactor:    0.001,
	})
}
