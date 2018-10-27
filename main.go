// Package main builds the OrderService application
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// HTTPResponseError is the common error response orderservice replies with to
// its callers on errors
type HTTPResponseError struct {
	Error string `json:"error"`
}

// CreateOrderDetails is the request body for a create order request.
type CreateOrderDetails struct {
	Origin      []string `json:"origin"`
	Destination []string `json:"destination"`
}

// GMapsDistance a struct in the GoogleMapsResponse
type GMapsDistance struct {
	Value int64  `json:"value"`
	Text  string `json:"text"`
}

// GoogleMapsResponse the HTTP response from a call to the distancematrix API
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
	// StateUnassigned represents an order that has been created
	StateUnassigned OrderState = "UNASSIGNED"
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
	mapsAPIKey string // Google Maps API Key, SECRET
	*http.ServeMux    // Embedded HTTP server object, implements http.Handler.
	*sql.DB           // Embedded SQL database connection.
}

// Insert adds a new entry to the database. origin and destination must be
// URL-encoded strings.
func (s *OrderService) Insert(details CreateOrderDetails) (*Order, error) {
	var (
		client = &http.Client{Timeout: 3 * time.Second}
		encode = func(input []string) string {
			return fmt.Sprintf("%s,%s", url.QueryEscape(input[0]), url.QueryEscape(input[1]))
		}
		origin      = encode(details.Origin)
		destination = encode(details.Destination)
	)

	url := fmt.Sprintf("https://maps.googleapis.com/maps/api/distancematrix/json?origins=%s&destinations=%s&key=%s",
		origin, destination, s.mapsAPIKey)
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed http.Client{}.Get() url=%s: %s", url, err)
	}
	defer response.Body.Close()

	var buf bytes.Buffer
	io.Copy(&buf, response.Body)

	fmt.Printf("Insert got response: %d. %s\n", response.StatusCode, buf.String())

	var mapResponse GoogleMapsResponse
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&mapResponse); err != nil {
		return nil, fmt.Errorf("unable to decode response: %s", err)
	}

	rowResult, err := s.DB.Exec("INSERT INTO orders (distance, status) values(?, ?)",
		mapResponse.Rows[0].Elements[0].Distance.Value, string(StateUnassigned))
	if err != nil {
		return nil, fmt.Errorf("unable to insert: %s", err)
	}
	lastId, err := rowResult.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("unable to insert, no row: %s", err)
	}

	return &Order{
		Id:       lastId,
		Distance: float64(mapResponse.Rows[0].Elements[0].Distance.Value),
		State:    StateUnassigned,
	}, nil
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
			orders = append(orders, Order{Id: id, Distance: distance, State: StateUnassigned})
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

		switch req.Method {
		case "GET":
			// default values.
			page, limit, err := parseQueryParametersForList(req.URL.Query())
			if err != nil {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INVALID_PARAMETERS"})
				return
			}
			orders, err := orderService.List(page, limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed orderService.List(): %s", err)
				w.WriteHeader(501)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INTERNAL_FAILURE"})
				return
			}
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(orders)
			return
		case "POST":
			var buf bytes.Buffer
			io.Copy(&buf, req.Body)
			req.Body.Close()
			fmt.Printf("post! bytes=%s\n", buf.String())

			details, err := parseCreateOrderDetails(buf.String())
			if err != nil {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: err.Error()})
				return
			}
			order, err := orderService.Insert(*details)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed orderService.Insert(): %s", err)
				w.WriteHeader(501)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INTERNAL_FAILURE"})
				return
			}
			json.NewEncoder(w).Encode(order)
			return
		default:
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(HTTPResponseError{Error: "INVALID_PARAMETERS"})
			return
		}
	})
	return orderService, nil
}

// On failure returns on errors. On success returns the "page" (first) and
// "limit" (second) parameters. Will provide default values for "page" and
// "limit" if no query parameters are provided.
func parseQueryParametersForList(qparams url.Values) (int, int, error) {
	// default values.
	var page int = 0
	var limit int = 50
	var err error
	if len(qparams["page"]) > 1 || len(qparams["limit"]) > 1 {
		return page, limit, err
	}
	if len(qparams["page"]) == 1 {
		page, err = strconv.Atoi(qparams["page"][0])
		if err != nil {
			return page, limit, err
		}
	}
	if len(qparams["limit"]) == 1 {
		page, err = strconv.Atoi(qparams["limit"][0])
		if err != nil {
			return page, limit, err
		}
	}
	return page, limit, nil
}

// parseCreateOrderDetails
func parseCreateOrderDetails(input string) (*CreateOrderDetails, error) {
	var details CreateOrderDetails
	if err := json.NewDecoder(strings.NewReader(input)).Decode(&details); err != nil {
		return nil, fmt.Errorf("MALFORMED_PAYLOAD")
	}
	if len(details.Origin) != 2 {
		return nil, fmt.Errorf("MALFORMED_ORIGIN")
	}
	if len(details.Destination) != 2 {
		return nil, fmt.Errorf("MALFORMED_DESTINATION")
	}
	return &details, nil
}

// Installs a signal handler and runs the service until interrupted. On
// graceful shutdown returns nil.
func orderServiceMain() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	var (
		dbname = flag.String("dbname", "", "Path to database")
	)
	flag.Parse()

	if *dbname == "" {
		return fmt.Errorf("missing db name")
	}
	db, err := sql.Open("sqlite3", *dbname)
	if err != nil {
		return fmt.Errorf("failed to open sqlite3 database (%s) : %s", *dbname, err)
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
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
