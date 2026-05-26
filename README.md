# README

Author: `Davis Wamola`

## How to run

The `Dockerfile` defines a self-contained Go reference environment. 
Build and run the program using [Docker](https://docs.docker.com/get-started/get-docker/):
```
$ docker build -t challenge .
$ docker run --rm -it challenge --auth=<token>
```

If go `1.26` or later is locally installed, run the program directly for convenience:
```
$ go run main.go --auth=<token>
```

## Assumptions

Single server with independent kitchen.
Order placing is idempotent so we will disregard duplicate orders if we find another order in storage. However, duplicates that arrive after pickup/expiry will still be re-placed.
No persistence model so every set of orders doesn't persist across multiple runs

## Discard criteria

**First to expire:** The shelf maintains an indexed min-heap keyed on each entry's projected `expiresAt`, so when room must be made we pop the entry closest to expiry in O(log n).

To maximises the chance that every other order on the shelf survives long enough to be picked up, we sacrifice the order the customer was least likely to receive in time anyway.

The same min-heap also drives the "move a shelf entry back to its ideal storage" path i.e. when several non-room entries are eligible to be relocated we pick the earliest-expiring among them, since that order benefits the most from
returning to its slower decay ideal storage type.

## Production readiness

Distributed service model with several kitchens can run on multiple servers without dependencies. I would use regional kitchen shards so that orders are handled by same server. However the lock will become the bottleneck at higher throughput.

Persistent storage would be used for recoverability and as well as observability rather than std out actions model. 

Better tracking of expired orders and pickup rates to reduce food waste.

Orders not found would return an error rather than fail silently.
