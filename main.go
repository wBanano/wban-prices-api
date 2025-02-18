package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const COINEX_API_URL = "https://api.coinex.com/v1"
const CACHE_TIME = 10 * time.Second

// Global cache variables.
var (
	cachedPrices  map[string]float64
	lastCacheTime time.Time
	cacheMutex    sync.Mutex
)

func main() {
	// Register the /prices route.
	http.HandleFunc("/prices", pricesHandler)

	// Catch-all handler for other paths.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		http.Error(w, "404", http.StatusNotFound)
	})

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func pricesHandler(w http.ResponseWriter, r *http.Request) {
	// Handle CORS pre-flight OPTIONS request.
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write([]byte("OK"))
		return
	}

	// Set headers for a successful JSON response.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Check if we have a valid cached result.
	cacheMutex.Lock()
	if time.Since(lastCacheTime) < CACHE_TIME && cachedPrices != nil {
		log.Println("/prices | CACHE HIT")
		cached := cachedPrices
		cacheMutex.Unlock()

		if err := json.NewEncoder(w).Encode(cached); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	cacheMutex.Unlock()

	// Cache miss: log and continue fetching new data.
	log.Println("/prices | CACHE MISS | Fetching new data")

	// Map of keys to CoinEx markets.
	markets := map[string]string{
		"ban":   "BANANOUSDT",
		"bnb":   "BNBUSDC",
		"eth":   "ETHUSDC",
		"matic": "POLUSDC",
		"ftm":   "SUSDC",
	}

	// Create a buffered channel to collect results.
	resultChan := make(chan PriceResult, len(markets))

	// Launch a goroutine for each market.
	for key, market := range markets {
		go func(key, market string) {
			price, err := getPrice(market)
			resultChan <- PriceResult{key: key, price: price, err: err}
		}(key, market)
	}

	// Collect results from the channel.
	prices := make(map[string]float64)
	for i := 0; i < len(markets); i++ {
		res := <-resultChan
		if res.err != nil {
			http.Error(w, res.err.Error(), http.StatusInternalServerError)
			return
		}
		prices[res.key] = res.price
	}

	// Update the cache with the new result.
	cacheMutex.Lock()
	cachedPrices = prices
	lastCacheTime = time.Now()
	cacheMutex.Unlock()

	// Encode and send the prices as JSON.
	if err := json.NewEncoder(w).Encode(prices); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func getPrice(market string) (float64, error) {
	url := fmt.Sprintf("%s%s%s", COINEX_API_URL, "/market/ticker?market=", market)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var tickerResp TickerResponse
	if err := json.Unmarshal(body, &tickerResp); err != nil {
		return 0, err
	}

	return strconv.ParseFloat(tickerResp.Data.Ticker.Last, 64)
}

type TickerResponse struct {
	Data struct {
		Ticker struct {
			Last string `json:"last"`
		} `json:"ticker"`
	} `json:"data"`
}

type PriceResult struct {
	key   string
	price float64
	err   error
}
