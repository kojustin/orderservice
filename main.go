// Package kojustin/orderservice builds the OrderService application
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
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const createOrderDetails = `{
  "origin": [
    "37.8093475",
    "-122.2740787"
  ],
  "destination": [
    "37.8061044",
    "-122.2943356"
  ]
}`

// HTTPResponseError is the common error response OrderService replies with to
// its callers on errors
type HTTPResponseError struct {
	Error string `json:"error"`
}

// HTTPResponseStatus is a response to some calls.
type HTTPResponseStatus struct {
	Status string `json:"status"`
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
	mapsAPIKey      string // Google Maps API Key, SECRET
	*http.ServeMux         // Embedded HTTP server object, implements http.Handler.
	*sql.DB                // Embedded SQL database connection.
	context.Context        // Context for cancelling and stuff.
	*http.Client           // HTTP Client
}

// Insert adds a new entry to the database. origin and destination must be
// URL-encoded strings.
func (s *OrderService) Insert(details CreateOrderDetails) (*Order, error) {
	var (
		encode = func(input []string) string {
			return fmt.Sprintf("%s,%s", url.QueryEscape(input[0]), url.QueryEscape(input[1]))
		}
		origin      = encode(details.Origin)
		destination = encode(details.Destination)
	)

	url := fmt.Sprintf("https://maps.googleapis.com/maps/api/distancematrix/json?origins=%s&destinations=%s&key=%s",
		origin, destination, s.mapsAPIKey)
	response, err := s.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed http.Client{}.Get() url=%s: %s", url, err)
	}
	defer response.Body.Close()

	debug := false
	var rdr io.Reader = response.Body
	if debug {
		var buf bytes.Buffer
		io.Copy(&buf, response.Body)
		fmt.Printf("Insert got response: %d. %s\n", response.StatusCode, buf.String())
		rdr = strings.NewReader(buf.String())
	}

	var mapResponse GoogleMapsResponse
	if err := json.NewDecoder(rdr).Decode(&mapResponse); err != nil {
		return nil, fmt.Errorf("unable to decode response: %s", err)
	}

	if len(mapResponse.Rows) == 0 {
		return nil, fmt.Errorf("Google Maps response missing rows")
	}
	firstRow := mapResponse.Rows[0]
	if len(firstRow.Elements) == 0 {
		return nil, fmt.Errorf("Google Maps response missing rows.elements")
	}
	firstElement := firstRow.Elements[0]

	rowResult, err := s.DB.Exec("INSERT INTO orders (distance, status) values(?, ?)",
		firstElement.Distance.Value, string(StateUnassigned))
	if err != nil {
		return nil, fmt.Errorf("unable to insert: %s", err)
	}
	lastId, err := rowResult.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("unable to insert, no row: %s", err)
	}

	return &Order{
		Id:       lastId,
		Distance: float64(firstElement.Distance.Value),
		State:    StateUnassigned,
	}, nil
}

// List returns a listing of orders.
func (s *OrderService) List(page int, limit int) ([]Order, error) {
	rows, err := s.DB.Query("SELECT id, distance, status FROM orders LIMIT ? OFFSET ?", limit, page)
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
		case string(StateTaken):
			orders = append(orders, Order{Id: id, Distance: distance, State: StateTaken})
		case string(StateUnassigned):
			orders = append(orders, Order{Id: id, Distance: distance, State: StateUnassigned})
		default:
			return nil, fmt.Errorf("found unknonwn status %s", status)
		}
	}

	return orders, nil
}

var (
	errTaken       = fmt.Errorf("already taken")
	errNoSuchOrder = fmt.Errorf("no such order")
)

// Take marks an order as taken. Returns errTaken if the order exists and has
// already been taken. Returns errNoSuchOrder if no such order exists. May
// return other errors.
func (s *OrderService) Take(orderID int64) error {
	ctx, cancelFn := context.WithTimeout(s.Context, 2*time.Second)
	defer cancelFn()

	var (
		err  error
		rows *sql.Rows
		tx   *sql.Tx
	)

	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed at BeginTx: %s", err)
	}
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	rows, err = tx.Query("SELECT (status) FROM orders where id == ?", orderID)
	if err != nil {
		return fmt.Errorf("unable to query for order ID: %s", err)
	}
	if rows.Next() == false {
		err = errNoSuchOrder
		return errNoSuchOrder
	}
	var status string
	err = rows.Scan(&status)
	if err != nil {
		err = fmt.Errorf("row.Scan() failed: %s", err)
		return err
	}

	if status != string(StateUnassigned) {
		err = errTaken
		return err
	}
	_, err = tx.Exec("UPDATE orders SET status = ? WHERE id = ?", string(StateTaken), orderID)
	if err != nil {
		return err
	}

	return nil
}

// NOOP assignment that verifies interface implementation.
var _ http.Handler = &OrderService{}

// NewOrderService creates a new OrderService object, registers handlers.
func NewOrderService(db *sql.DB, mapsAPIKey string, ctx context.Context) (*OrderService, error) {
	mux := http.NewServeMux()
	orderService := &OrderService{mapsAPIKey: mapsAPIKey, ServeMux: mux, DB: db, Context: ctx, Client: &http.Client{Timeout: 3 * time.Second}}

	patchPathRE, err := regexp.Compile("^/orders/(?P<orderID>[[:digit:]]*)$")
	if err != nil {
		return nil, fmt.Errorf("unable to compile patchPathRE: %s", err)
	}

	mux.HandleFunc("/orders/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "PATCH" {
			// Allow only PATCH. Otherwise, return 405 Method Not Allowed
			fmt.Printf("Method:%s; Path:%s, 405\n", req.Method, req.URL.Path)
			w.WriteHeader(405)
			json.NewEncoder(w).Encode(HTTPResponseError{"DISALLOWED_METHOD"})
			return
		}

		matches := patchPathRE.FindStringSubmatch(req.URL.Path)
		if len(matches) != 2 {
			// Only allow URLS like "/orders/ID" where ID is an integer.
			// Otherwise, return 404 not found.
			fmt.Printf("Method:%s; Path:%s, 404 no matches\n", req.Method, req.URL.Path)
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(HTTPResponseError{"NO_SUCH_ORDER"})
			return
		}
		orderID, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			fmt.Printf("Method:%s; Path:%s, 400 invalid id\n", req.Method, req.URL.Path)
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(HTTPResponseError{"INVALID_ORDER_ID"})
			return
		}
		switch err = orderService.Take(orderID); err {
		case errNoSuchOrder:
			fmt.Printf("Method:%s; Path:%s, 404 no such order %d\n", req.Method, req.URL.Path, orderID)
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(HTTPResponseError{"NO_SUCH_ORDER"})
			return
		case errTaken:
			fmt.Printf("Method:%s; Path:%s, 409 order %d already taken\n", req.Method, req.URL.Path, orderID)
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(HTTPResponseError{"ORDER_ALREADY_BEEN_TAKEN"})
			return
		case nil:
			fmt.Printf("Method:%s; Path:%s, 200 order %d success\n", req.Method, req.URL.Path, orderID)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(HTTPResponseStatus{"SUCCESS"})
			return
		default:
			fmt.Printf("Method:%s; Path:%s, 500 orderService.Take() %d failed: %s\n", req.Method, req.URL.Path,
				orderID, err)
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(HTTPResponseError{"INTERNAL_ERROR"})
			return
		}
	})

	mux.HandleFunc("/orders", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/orders" {
			fmt.Printf("Method:%s; Path:%s, 404\n", req.Method, req.URL.Path)
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(HTTPResponseError{"INVALID_PATH"})
			return
		}

		switch req.Method {
		case "GET":
			// default values.
			page, limit, err := parseQueryParametersForList(req.URL.Query())
			if err != nil {
				fmt.Printf("Method:%s; Path:%s, 400 invalid params\n", req.Method, req.URL.Path)
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INVALID_PARAMETERS"})
				return
			}
			orders, err := orderService.List(page, limit)
			if err != nil {
				fmt.Printf("Method:%s; Path:%s, 500 failed orderService.List(): %s\n",
					req.Method, req.URL.Path, err)
				w.WriteHeader(500)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INTERNAL_FAILURE"})
				return
			}
			fmt.Printf("Method:%s; Path:%s, 200 page=%d limit=%d\n", req.Method, req.URL.Path, page, limit)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(orders)
			return
		case "POST":
			var buf bytes.Buffer
			io.Copy(&buf, req.Body)

			details, err := parseCreateOrderDetails(buf.String())
			if err != nil {
				fmt.Printf("Method:%s; Path:%s, 400 parseCreateOrderDetails(): %s\n",
					req.Method, req.URL.Path, err)
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: err.Error()})
				return
			}
			order, err := orderService.Insert(*details)
			if err != nil {
				fmt.Printf("Method:%s; Path:%s, 500 orderService.Insert(): %s\n", req.Method, req.URL.Path, err)
				w.WriteHeader(500)
				json.NewEncoder(w).Encode(HTTPResponseError{Error: "INTERNAL_FAILURE"})
				return
			}
			fmt.Printf("Method:%s; Path:%s, 200 post order success %+v\n", req.Method, req.URL.Path, order)
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(order)
			return
		default:
			fmt.Printf("Method:%s; Path:%s, 400 invalid params \n", req.Method, req.URL.Path)
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(HTTPResponseError{Error: "INVALID_PARAMETERS"})
			return
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Printf("Method:%s; Path:%s, 404 default handler\n", req.Method, req.URL.Path)
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(HTTPResponseError{"INVALID_PATH"})
		return
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
		limit, err = strconv.Atoi(qparams["limit"][0])
		if err != nil {
			return page, limit, err
		}
	}
	return page, limit, nil
}

// parseCreateOrderDetails returns non-nil error on failure
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
		ctx    = context.Background()
		dbpath = flag.String("dbpath", "", "Path to database")
		port   = flag.Int("port", 8080, "Port number to listen on")
	)
	flag.Parse()

	if *dbpath == "" {
		return fmt.Errorf("missing db name")
	}
	db, err := sql.Open("sqlite3", *dbpath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite3 database (%s) : %s", *dbpath, err)
	}
	defer db.Close()

	mapsAPIKey, ok := os.LookupEnv("GOOGLE_MAPS_API_KEY")
	if !ok {
		return fmt.Errorf("missing environment variable GOOGLE_MAPS_API_KEY")
	}
	if mapsAPIKey == "" {
		return fmt.Errorf("environment variable GOOGLE_MAPS_API_KEY is empty")
	}

	orderService, err := NewOrderService(db, mapsAPIKey, ctx)
	if err != nil {
		return fmt.Errorf("failed to create OrderService: %s", err)
	}

	server := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: orderService}

	go func() {
		for _ = range c {
			ctx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
			defer cancelFn()
			server.Shutdown(ctx)
		}
	}()

	// Serve traffic. If we were closed by a graceful shutdown (e.g. caught
	// a Ctrl+C) don't return an error.
	fmt.Printf("Listening.")
	serveErr := server.ListenAndServe()
	if serveErr == http.ErrServerClosed {
		fmt.Fprintf(os.Stdout, "\nSignal caught, exiting.\n")
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
