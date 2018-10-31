// +build !integ

package main

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// Literal from (https://developers.google.com/maps/documentation/distance-matrix/intro#DistanceMatrixResponses).
const gmapsResponse = `{
  "status": "OK",
  "origin_addresses": [ "Vancouver, BC, Canada", "Seattle, État de Washington, États-Unis" ],
  "destination_addresses": [ "San Francisco, Californie, États-Unis", "Victoria, BC, Canada" ],
  "rows": [ {
    "elements": [ {
      "status": "OK",
      "duration": {
        "value": 340110,
        "text": "3 jours 22 heures"
      },
      "distance": {
        "value": 1734542,
        "text": "1 735 km"
      }
    }, {
      "status": "OK",
      "duration": {
        "value": 24487,
        "text": "6 heures 48 minutes"
      },
      "distance": {
        "value": 129324,
        "text": "129 km"
      }
    } ]
  }, {
    "elements": [ {
      "status": "OK",
      "duration": {
        "value": 288834,
        "text": "3 jours 8 heures"
      },
      "distance": {
        "value": 1489604,
        "text": "1 490 km"
      }
    }, {
      "status": "OK",
      "duration": {
        "value": 14388,
        "text": "4 heures 0 minutes"
      },
      "distance": {
        "value": 135822,
        "text": "136 km"
      }
    } ]
  } ]
}`

func TestDeserializeGoogleMapsResponse(t *testing.T) {
	var response GoogleMapsResponse
	err := json.NewDecoder(strings.NewReader(gmapsResponse)).Decode(&response)
	if err != nil {
		t.Errorf("decode gmapsResponse failed %s", err)
	}

	if lenRows := len(response.Rows); lenRows != 2 {
		t.Errorf("expected 2 rows, got %d", lenRows)
	}
	rowZero := response.Rows[0]
	if lenElems := len(rowZero.Elements); lenElems != 2 {
		t.Errorf("expected 2 elements, got %d", lenElems)
	}

	distance1 := GMapsDistance{Value: 1734542, Text: "1 735 km"}
	if !reflect.DeepEqual(rowZero.Elements[0].Distance, distance1) {
		t.Errorf("expected %+v, got %+v", distance1, rowZero.Elements[0])
	}
}

func TestDeserializeCreateOrderDetails(t *testing.T) {
	details, err := parseCreateOrderDetails(createOrderDetails)
	if err != nil {
		t.Errorf("parseCreateOrderDetails failed: %s", err)
	}
	t.Logf("details=%+v", details)

}

func TestParseQueryParametersForList(t *testing.T) {
	qparams := url.Values{}
	qparams["page"] = []string{"3"}
	qparams["limit"] = []string{"5"}

	p, l, err := parseQueryParametersForList(qparams)
	if err != nil {
		t.Error(err)
	}
	if p != 3 {
		t.Error(p)
	}
	if l != 5 {
		t.Error(l)
	}

	p, l, err = parseQueryParametersForList(url.Values{})
	if p != 1 || l != 10 || err != nil {
		t.Error(p, l, err)
	}
}
