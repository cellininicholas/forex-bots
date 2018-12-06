package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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

type botDatum struct {
	Name    string    `json:"name"`
	Account string    `json:"account"`
	Params  botParams `json:"params"`
}

type botData []botDatum

type botParams struct {
	CandleGranularity string  `json:"candleGranularity"`
	CandleCount       string  `json:"candleCount"`
	MomentumPeriod    int     `json:"momentumPeriod"`
	SMAPeriod         int     `json:"SMAPeriod"`
	RsiPeriod         int     `json:"rsiPeriod"`
	StDevPeriod       int     `json:"stDevPeriod"`
	VolumeFactor      float64 `json:"volumeFactor"`
	TakeProfitFactor  float64 `json:"takeProfitFactor"`
	StopLossFactor    float64 `json:"stopLossFactor"`
}

func startOandaConnection(accountID string) (*goanda.OandaConnection, string) {
	// Load the env
	if godotenv.Load() != nil { // err
		log.Fatal("No api keys")
	}
	key := os.Getenv("OANDA_API_KEY")
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

func averagePointer(candles *[]goanda.Candle) (result float64) {
	for _, c := range *candles {
		result += c.Mid.Close
	}
	result /= math.Max(float64(len(*candles)), 1)
	return
}

func average(candles []goanda.Candle) (result float64) {
	result = averagePointer(&candles)
	return
}

func computeMomentum(candles *goanda.BidAskCandles, period int) float64 {
	endIndex := len(candles.Candles) - 1
	var halfPeriod int
	if period > endIndex {
		halfPeriod = endIndex / 2
	} else {
		halfPeriod = period / 2
	}
	firstHalf := candles.Candles[endIndex-2*halfPeriod : endIndex-halfPeriod]
	lastHalf := candles.Candles[endIndex-halfPeriod : endIndex]
	return averagePointer(&lastHalf) - averagePointer(&firstHalf)
}

func computeRSI(candles *goanda.BidAskCandles, period int) float64 {
	return 1
}

func computeStandardDeviation(candles *goanda.BidAskCandles, period int) float64 {
	return 2.0
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(int(num*output)) / output
}

func placeOrder(connection *goanda.OandaConnection, instrument *goanda.Instrument, units int, takeProfit float64, stopLoss float64) *goanda.OrderResponse {
	// Use precision of instrument to constuct tp and sl string
	prec := instrument.DisplayPrecision
	tp := toFixed(takeProfit, prec)
	sl := toFixed(stopLoss, prec)
	response := connection.CreateOrder(goanda.OrderPayload{Order: goanda.OrderBody{
		Instrument:       instrument.Name,
		Type:             market,
		Units:            units,
		TimeInForce:      fireOrKill,
		PositionFill:     defaultFill,
		TakeProfitOnFill: &goanda.OnFill{Price: fmt.Sprintf("%f", tp)},
		StopLossOnFill:   &goanda.OnFill{Price: fmt.Sprintf("%f", sl)},
	}})
	return &response
}

func goLong(connection *goanda.OandaConnection, instrument *goanda.Instrument, lastCandle *goanda.Candle, params *botParams) {
	tradeVol := int((1 / lastCandle.Bid.Close) * params.VolumeFactor)
	if tradeVol <= 0 {
		return
	}
	tp := lastCandle.Ask.Close + lastCandle.Ask.Close*params.TakeProfitFactor
	sl := lastCandle.Bid.Close - lastCandle.Bid.Close*params.StopLossFactor
	placeOrder(connection, instrument, tradeVol, tp, sl)
}

func goShort(connection *goanda.OandaConnection, instrument *goanda.Instrument, lastCandle *goanda.Candle, params *botParams) {
	tradeVol := int((1 / lastCandle.Ask.Close) * params.VolumeFactor)
	if tradeVol <= 0 {
		return
	}
	tp := lastCandle.Bid.Close - lastCandle.Bid.Close*params.TakeProfitFactor
	sl := lastCandle.Ask.Close + lastCandle.Ask.Close*params.StopLossFactor
	placeOrder(connection, instrument, -tradeVol, tp, sl)
}

func analyseAndTrade(connection *goanda.OandaConnection, instrument goanda.Instrument, params *botParams, wg *sync.WaitGroup) {
	defer wg.Done()

	candles := connection.GetBidAskCandles(instrument.Name, params.CandleCount, params.CandleGranularity)
	lastIndex := len(candles.Candles) - 1
	// Assess the candles for statistics
	momentum := computeMomentum(&candles, params.MomentumPeriod)
	// rsi := computeRSI(&candles, params.rsiPeriod)
	simpleAverage := average(candles.Candles[lastIndex-params.SMAPeriod:])
	lastCandle := candles.Candles[lastIndex]
	// Use std dev to find the bolinger band, we can then know how far away from
	// the mean we should be, and how far to set to TP
	// stDev := computeStandardDeviation(&candles, params.stDevPeriod)
	// If score is high, enter following block
	if simpleAverage > lastCandle.Ask.Close && momentum > 0 {
		goLong(connection, &instrument, &lastCandle, params)
	} else if simpleAverage < lastCandle.Bid.Close && momentum < 0 {
		goShort(connection, &instrument, &lastCandle, params)
	}
}

func stripParse(s string, p string) int64 {
	i, err := strconv.ParseInt(strings.Replace(s, p, "", 1), 0, 0)
	if err != nil {
		log.Fatal("Bad granularity format")
	}
	return i
}

func granularityToDuration(granularity string) time.Duration {
	if strings.HasPrefix(granularity, "S") {
		return time.Second * time.Duration(stripParse(granularity, "S"))
	}
	if strings.HasPrefix(granularity, "M") {
		return time.Minute * time.Duration(stripParse(granularity, "M"))
	}
	if strings.HasPrefix(granularity, "H") {
		return time.Hour * time.Duration(stripParse(granularity, "H"))
	}
	if strings.HasPrefix(granularity, "D") {
		return time.Hour * time.Duration(24) * time.Duration(stripParse(granularity, "D"))
	}
	return time.Minute
}

// runBot can be run on its own thread to allow testing bots simultaneously
func runBot(botDatum botDatum, wg *sync.WaitGroup) {
	defer wg.Done()
	connection, accountID := startOandaConnection(botDatum.Account)
	instruments := getCurrencies(connection, accountID)
	params := &botDatum.Params
	for {
		noPos := getInstrumentsWithoutPositions(connection, instruments)
		fmt.Println("Running", botDatum.Name, "on", len(noPos.Instruments), "instruments")
		var wg sync.WaitGroup
		wg.Add(len(noPos.Instruments))
		for _, instrument := range noPos.Instruments {
			// Get the candles of interest
			go analyseAndTrade(connection, instrument, params, &wg)
		}
		wg.Wait()
		newNoPos := getInstrumentsWithoutPositions(connection, instruments)
		fmt.Println("Placed", len(noPos.Instruments)-len(newNoPos.Instruments), "trades")
		fmt.Println("Waiting for next candle")
		time.Sleep(granularityToDuration(params.CandleGranularity))
	}
}

func loadBots() (bots botData) {
	fmt.Println("Loading bots")
	file, err := os.Open("bots.json")
	if err != nil {
		log.Fatal("Missing bots.json")
	}
	defer file.Close()
	byteValue, _ := ioutil.ReadAll(file)
	json.Unmarshal(byteValue, &bots)
	return
}

func main() {
	bots := loadBots()
	var wg sync.WaitGroup
	for _, bot := range bots {
		wg.Add(1)
		go runBot(bot, &wg)
	}
	wg.Wait()
}
