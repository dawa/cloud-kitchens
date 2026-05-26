package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	urlpkg "net/url"
	"time"
)

// Order is a json-friendly representation of an order.
type Order struct {
	ID        string `json:"id"`        // order id
	Name      string `json:"name"`      // food name
	Temp      string `json:"temp"`      // ideal temperature
	Price     int    `json:"price"`     // price in dollars
	Freshness int    `json:"freshness"` // freshness in seconds
}

// Action names
const (
	Place   = "place"
	Move    = "move"
	Pickup  = "pickup"
	Discard = "discard"
)

// Target names
const (
	Heater = "heater"
	Cooler = "cooler"
	Shelf  = "shelf"
)

// Action is a json-friendly representation of an action.
type Action struct {
	Timestamp int64  `json:"timestamp"` // unix timestamp in microseconds
	ID        string `json:"id"`        // order id
	Action    string `json:"action"`    // place, move, pickup or discard
	Target    string `json:"target"`    // heater, cooler or shelf. Target is the destination for move
}

type options struct {
	Rate int64 `json:"rate"` // inverse rate in microseconds
	Min  int64 `json:"min"`  // min pickup in microseconds
	Max  int64 `json:"max"`  // max pickup in microseconds
}

type solution struct {
	Options options  `json:"options"`
	Actions []Action `json:"actions"`
}

// Client is a client for fetching and solving challenge test problems.
type Client struct {
	endpoint, auth string
}

func NewClient(endpoint, auth string) *Client {
	return &Client{endpoint: endpoint, auth: auth}
}

// New fetches a new test problem from the server. The URL also works in a browser for convenience.
func (c *Client) New(name string, seed int64) (string, []Order, error) {
	if seed == 0 {
		seed = rand.New(rand.NewSource(time.Now().UnixNano())).Int63()
	}

	url, err := urlpkg.Parse(fmt.Sprintf("%v/interview/challenge/new", c.endpoint))
	if err != nil {
		return "", nil, err
	}

	url.RawQuery = urlpkg.Values{"auth": {c.auth}, "name": {name}, "seed": {fmt.Sprintf("%d", seed)}}.Encode()

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return "", nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%v: %v: %s", url, resp.Status, string(buf))
	}

	var orders []Order
	if err := json.Unmarshal(buf, &orders); err != nil {
		return "", nil, fmt.Errorf("failed to deserialize '%v': %v", string(buf), err)
	}
	id := resp.Header.Get("x-test-id")

	log.Printf("Fetched new test problem, id=%v: %v", id, url)
	return id, orders, nil
}

// Solve submits a sequence of actions and parameters as a solution to a test problem. Returns test result.
func (c *Client) Solve(id string, rate, min, max time.Duration, actions []Action) (string, error) {
	url, err := urlpkg.Parse(fmt.Sprintf("%v/interview/challenge/solve", c.endpoint))
	if err != nil {
		return "", err
	}

	url.RawQuery = urlpkg.Values{"auth": {c.auth}}.Encode()

	payload := solution{
		Options: options{
			Rate: rate.Microseconds(),
			Min:  min.Microseconds(),
			Max:  max.Microseconds(),
		},
		Actions: actions,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Add("x-test-id", id)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%v: %v: %s", url, resp.Status, string(buf))
	}

	return string(buf), nil
}
