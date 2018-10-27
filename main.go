// Package main builds the OrderService application
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type HTTPResponseError struct {
	Error string `json:"error"`
}

type GMapsDistance struct {
	Value int64  `json:"value"`
	Text  string `json:"text"`
}

type GoogleMapsResponse struct {
	Rows []struct {
		Elements []struct {
			Distance GMapsDistance `json:"distance"`
		} `json:"elements"`
	} `json:"rows"`
}

// OrderState is a type alias.
type OrderState = string

const (
	// StateInitial represents an order that has been created
	StateInitial OrderState = "INIT"
	// StateTaken represents an order that has been "taken" or assigned.
	StateTaken = "TAKEN"
)

// Order represents an order in the system. This is exactly the same schema as
// rows in the database.
type Order struct {
	Id       int64      `json:"id"`
	Distance float64    `json:"distance"`
	State    OrderState `json:"status"`
}

// OrderService is a net/http.Handler that deals with orders.
type OrderService struct {
	mapsAPIKey     string // Google Maps API Key, SECRET
	*http.ServeMux        // Embedded HTTP server object, implements http.Handler.
	*sql.DB               // Embedded SQL database connection.
}

// Insert adds a new entry to the database. origin and destination must be
// URL-encoded strings.
func (s *OrderService) Insert(origin string, destination string) (*Order, error) {
	client := &http.Client{Timeout: 3*time.Second}

	url := fmt.Sprintf("https://maps.googleapis.com/maps/api/distancematrix/json?originss=%s&destinations=%s&key=%s",
		origin, destination, s.mapsAPIKey)
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed http.Client{}.Get() url=%S: %s", url, err)
	}
	defer response.Body.Close()

	var buf bytes.Buffer
	io.Copy(&buf, response.Body)

	fmt.Printf("Insert got response: %d. %s\n", response.StatusCode, buf.String())

	var mapResponse GoogleMapsResponse
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&mapResponse); err != nil {
		return nil, fmt.Errorf("unable to decode response: %s", err)
	}



	return nil, nil
}

// List returns a listing of orders.
func (s *OrderService) List(page int, limit int) ([]Order, error) {
	rows, err := s.DB.Query("SELECT id, distance, status FROM orders")
	if err != nil {
		return nil, fmt.Errorf("SELECT ... FROM failed: %s", err)
	}
	defer rows.Close()

	orders := []Order{}

	for rows.Next() {
		var id int64
		var distance float64
		var status string

		err = rows.Scan(&id, &distance, &status)
		if err != nil {
			return nil, fmt.Errorf("row.Scan() failed: %s", err)
		}
		switch status {
		case "TAKEN":
			orders = append(orders, Order{Id: id, Distance: distance, State: StateTaken})
		case "INITAL":
			orders = append(orders, Order{Id: id, Distance: distance, State: StateInitial})
		default:
			return nil, fmt.Errorf("found unknonwn status %s", status)
		}
	}

	return orders, nil
}

// NOOP assignment that verifies interface implementation.
var _ http.Handler = &OrderService{}

// NewOrderService creates a new OrderService object, registers handlers.
func NewOrderService(db *sql.DB, mapsAPIKey string) (*OrderService, error) {
	mux := http.NewServeMux()
	orderService := &OrderService{mapsAPIKey: mapsAPIKey, ServeMux: mux, DB: db}

	mux.HandleFunc("/orders", func(w http.ResponseWriter, req *http.Request) {
		fmt.Printf("req.URL.Path:%s, RequestURI:%s; method=%s\n", req.URL.Path, req.RequestURI, req.Method)

		if req.Method == "GET" {
			qparams := req.URL.Query()

			errResponse := HTTPResponseError{Error: "INVALID_PARAMETERS"}
			sendErr := func() {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(errResponse)
			}

			// default values.
			var page int = 0
			var limit int = 50
			var err error
			if len(qparams["page"]) > 1 || len(qparams["limit"]) > 1 {
				sendErr()
				return
			}
			if len(qparams["page"]) == 1 {
				page, err = strconv.Atoi(qparams["page"][0])
				if err != nil {
					sendErr()
					return
				}
			}
			if len(qparams["limit"]) == 1 {
				page, err = strconv.Atoi(qparams["limit"][0])
				if err != nil {
					sendErr()
					return
				}
			}
			orders, err := orderService.List(page, limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed orderService.List(): %s", err)
				w.WriteHeader(501)
				json.NewEncoder(w).Encode(errResponse)
				return
			}
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(orders)

		}
	})
	return orderService, nil
}

// Installs a signal handler and runs the service until interrupted. On
// graceful shutdown returns nil.
func orderServiceMain() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	dbname := "orders.db"
	db, err := sql.Open("sqlite3", "orders.db")
	if err != nil {
		return fmt.Errorf("failed to open sqlite3 database (%s) : %s", dbname, err)
	}
	fmt.Printf("opened db.\n")
	defer db.Close()

	mapsAPIKey, ok := os.LookupEnv("GOOGLE_MAPS_API_KEY")
	if !ok {
		return fmt.Errorf("missing environment variable GOOGLE_MAPS_API_KEY")
	}
	if mapsAPIKey == "" {
		return fmt.Errorf("environment variable GOOGLE_MAPS_API_KEY is empty")
	}

	orderService, err := NewOrderService(db, mapsAPIKey)
	if err != nil {
		return fmt.Errorf("failed to create OrderService: %s", err)
	}

	server := &http.Server{Addr: ":8080", Handler: orderService}

	go func() {
		for _ = range c {
			ctx, cancelFn := context.WithTimeout(context.Background(),
				5*time.Second)
			defer cancelFn()
			server.Shutdown(ctx)
		}
	}()

	// Serve traffic. If we were closed by a graceful shutdown (e.g. caught
	// a Ctrl+C) don't return an error.
	serveErr := server.ListenAndServe()
	if serveErr == http.ErrServerClosed {
		fmt.Fprintf(os.Stdout, "shutdown.\n")
		return nil
	}
	return serveErr
}

func main() {
	if err := orderServiceMain(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s", err)
		os.Exit(1)
	}
}
