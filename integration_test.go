// This test file has the tag "integ" and implements the integration tests.
// +build integ

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

const contentType = "application/json"

var (
	svcHostNameFlag = flag.String("svcHostName", "localhost:8081", "Hostname to use when connecting to the service")
)

func TestMain(m *testing.M) {
	fmt.Printf("Running integ test, exits with non-zero exit code if tests fail.\n")
	fmt.Printf("Tests should run with a empty database.\n")

	flag.Parse()

	fmt.Printf("Using hostname '%s'.\n", *svcHostNameFlag)
	os.Exit(m.Run())
}

func TestIntegration(t *testing.T) {
	c := http.Client{}

	// The test requires that the db start off empty. Empty db should return
	// zero-length list.
	assertEmptyList(t, c)

	// Try to insert an invalid item, check that it fails.
	assertInsertMalformedFailure(t, c)

	// Insert an item, check that the item returns the right response.
	assertInsertSuccess(t, c)

	// Taking a non-existent item should fail.
	assertTakeNonExistentFails(t, c)

	// Take the first item and check that it succeeds.
	assertTakeSuccess(t, c)

	// Take a the first item (again) and check it fails.
	assertTakeAgainFails(t, c)

	// Insert a bunch of journeys that are not exactly the same.
	originLat := 37.8093475    // North-South
	originLong := -122.2740787 // East-West
	for idx := 0; idx < 14; idx++ {

		newLat := originLat + (float64(idx) * 0.02)
		newLong := originLong - (float64(idx) * 0.02)

		createOrderDetails := CreateOrderDetails{
			Origin:      []string{fmt.Sprintf("%f", newLat), fmt.Sprintf("%f", newLong)},
			Destination: []string{"37.8061044", "-122.2943356"},
		}
		insertOrder(t, c, createOrderDetails)
	}

	// Fetch some items from the middle to test pagination.
	items := getList(t, c, 3, 3)
	if len(items) != 3 {
		t.Error(len(items))
	}

	expectedResult := []Order{
		{Id: 4, Distance: 6601, State: "UNASSIGNED"},
		{Id: 5, Distance: 9318, State: "UNASSIGNED"},
		{Id: 6, Distance: 22475, State: "UNASSIGNED"},
	}
	for idx, elem := range items {
		expected := expectedResult[idx]
		if elem.Id != expected.Id {
			t.Error(idx, elem.Id, expected.Id)
		}
		if elem.Distance != expected.Distance {
			t.Error(idx, elem.Distance, expected.Distance)
		}
		if elem.State != expected.State {
			t.Error(idx, elem.State, expected.State)
		}
	}
}

func assertEmptyList(t *testing.T, client http.Client) {
	resp, err := client.Get(fmt.Sprintf("http://%s/orders", *svcHostNameFlag))
	if err != nil {
		t.Errorf("GET /orders failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("GET /orders returned %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, resp.Body); err != nil {
		t.Errorf("ioCopy %s", err)
	} else if buf.String() != "[]\n" {
		t.Errorf("expected empty array, got '%s'", buf.String())
	}
}

func assertInsertMalformedFailure(t *testing.T, client http.Client) {
	resp, err := client.Post(fmt.Sprintf("http://%s/orders", *svcHostNameFlag), contentType, strings.NewReader("malformed"))
	if err != nil {
		t.Errorf("POST /orders failed: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Errorf("POST /orders should have failed %d", resp.StatusCode)
	}
}

func assertInsertSuccess(t *testing.T, client http.Client) {
	resp, err := client.Post(fmt.Sprintf("http://%s/orders", *svcHostNameFlag), contentType, strings.NewReader(createOrderDetails))
	if err != nil {
		t.Errorf("POST /orders failed: %s", err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("POST /orders returned %d", resp.StatusCode)
	}
	var order Order
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&order); err != nil {
		t.Errorf("POST /orders response body malformed")
	}
	if order.Id != 1 || order.Distance != 2489 || order.State != string(StateUnassigned) {
		t.Errorf("POST /orders incorrect response, %+v", order)
	}
}

func assertTakeNonExistentFails(t *testing.T, client http.Client) {
	patchRequest, err := http.NewRequest("PATCH", fmt.Sprintf("http://%s/orders/23", *svcHostNameFlag), nil)
	if err != nil {
		t.Error(err)
	}
	resp, err := client.Do(patchRequest)
	if resp.StatusCode == 200 {
		t.Error("success on non-existent order")
	}
}

func assertTakeSuccess(t *testing.T, client http.Client) {
	patchRequest, err := http.NewRequest("PATCH", fmt.Sprintf("http://%s/orders/1", *svcHostNameFlag), nil)
	if err != nil {
		t.Error(err)
	}
	resp, err := client.Do(patchRequest)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Error(resp.StatusCode)
	}
	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)

	var status HTTPResponseStatus
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&status); err != nil {
		t.Errorf("PATCH /orders/1 response body malformed")
	}
	if status.Status != "SUCCESS" {
		t.Errorf("PATCH /orders/1 incorrect response, %+v", status)
	}
}

func assertTakeAgainFails(t *testing.T, client http.Client) {
	patchRequest, err := http.NewRequest("PATCH", fmt.Sprintf("http://%s/orders/1", *svcHostNameFlag), nil)
	if err != nil {
		t.Error(err)
	}
	resp, err := client.Do(patchRequest)
	if err != nil {
		t.Error(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error(resp.StatusCode)
	}
	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)

	var httpErr HTTPResponseError
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&httpErr); err != nil {
		t.Errorf("PATCH /orders/1 response body malformed")
	}
	if httpErr.Error != "ORDER_ALREADY_BEEN_TAKEN" {
		t.Errorf("PATCH /orders/1 incorrect response, %+v", httpErr)
	}
}

// Inserts over HTTP
func insertOrder(t *testing.T, client http.Client, createOrder CreateOrderDetails) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(createOrder); err != nil {
		t.Errorf("unable to encode %+v: %s", createOrder, err)
	}
	resp, err := client.Post(fmt.Sprintf("http://%s/orders", *svcHostNameFlag), contentType, strings.NewReader(buf.String()))
	if err != nil {
		t.Errorf("POST /orders failed: %s", err)
	}
	defer resp.Body.Close()
	buf.Reset()
	io.Copy(&buf, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("POST /orders returned %d", resp.StatusCode)
	}
	var order Order
	if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(&order); err != nil {
		t.Errorf("POST /orders response body malformed")
	}
}

func getList(t *testing.T, client http.Client, page int, limit int) []Order {
	resp, err := client.Get(fmt.Sprintf("http://%s/orders?page=%d&limit=%d", *svcHostNameFlag, page, limit))
	if err != nil {
		t.Errorf("GET /orders failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("GET /orders returned %d", resp.StatusCode)
	}

	var orders []Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		t.Error("unable to decode items.")
	}
	return orders
}
