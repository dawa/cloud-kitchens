package main

import (
	"flag"
	"log"
	"time"

	"challenge/client"
	"challenge/harness"
	"challenge/kitchen"
)

var (
	endpoint = flag.String("endpoint", "https://api.cloudkitchens.com", "Problem server endpoint")
	auth     = flag.String("auth", "", "Authentication token (required)")
	name     = flag.String("name", "", "Problem name. Leave blank (optional)")
	seed     = flag.Int64("seed", 0, "Problem seed (random if zero)")

	rate = flag.Duration("rate", 500*time.Millisecond, "Inverse order rate")
	min  = flag.Duration("min", 4*time.Second, "Minimum pickup time")
	max  = flag.Duration("max", 8*time.Second, "Maximum pickup time")
)

func main() {
	flag.Parse()

	c := client.NewClient(*endpoint, *auth)
	id, orders, err := c.New(*name, *seed)
	if err != nil {
		log.Fatalf("Failed to fetch test problem: %v", err)
	}

	k := kitchen.New()
	actions := harness.Run(k, orders, *rate, *min, *max)

	result, err := c.Solve(id, *rate, *min, *max, actions)
	if err != nil {
		log.Fatalf("Failed to submit test solution: %v", err)
	}
	log.Printf("Test result: %v", result)
}
