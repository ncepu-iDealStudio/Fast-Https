package appfirewall

import (
	"fast-https/modules/core/request"
	"fmt"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Config struct for passing options
type XssMw struct {
	// List of fields to not filter. i.e. password, created_on, created_at, etc
	// password is set to skip by the system i.e. FieldsToSkip = []string{"password", "cre_date"}
	FieldsToSkip []string

	// TODO: need more granular skipping...
	// List of tables to not filter any fields on
	// how would you know this, coming from front end forms/params?
	//TablesToSkip []string

	// Hash of table->field combinations to skip filtering on
	//TableFieldRelationToSkip map[string]string

	// Bluemonday comes with two default policies
	// Two options StrictPolicy // the default
	//             UGCPolicy
	// or you can specify you own policy
	// define it somewhere in your package so that you can call it here
	// see https://github.com/microcosm-cc/bluemonday/blob/master/policies.go
	// This must contain one of three possible settings:
	//             StrictPolicy // the default
	//             UGCPolicy
	//             New          // Specify your own policy - not yet supported
	BmPolicy string
}

type XssMwJson map[string]interface{}

// Get which Bluemonday policy
func (mw *XssMw) GetBlueMondayPolicy() *bluemonday.Policy {

	if mw.BmPolicy == "UGCPolicy" {
		return bluemonday.UGCPolicy()
		//} else if mw.BmPolicy == "New" {
		//	// TODO: will have to construct one with settings passed
		//	fmt.Println("New Not Yet Implemented!")
	}

	// default
	return bluemonday.StrictPolicy()
}

func HandleXss(req *request.Request) bool {
	fmt.Println("This is appfirewall, handle sql")
	xss := XssMw{}
	xss.XssRemove(req)
	return true
}

// Receives an http request object, processes the body, removing html and returns the request.
//
// Headers (and other parts of the request) are passed through unaltered.
//
// Request Method must be "POST" or "PUT"
func (mw *XssMw) XssRemove(req *request.Request) error {
	// https://golang.org/pkg/net/http/#Request

	ReqMethod := req.Method

	// [application/json] only supported
	ctHdr := req.GetContentType()

	ctsLen := req.GetContentLength()

	ctLen, _ := strconv.Atoi(ctsLen)

	// https://golang.org/src/net/http/request.go
	// check expected application type
	if ReqMethod == "POST" || ReqMethod == "PUT" || ReqMethod == "PATCH" {

		if ctLen > 1 && ctHdr == "application/json" {
		} else if ctHdr == "application/x-www-form-urlencoded" {
		} else if strings.Contains(ctHdr, "multipart/form-data") {
		}
	} else if ReqMethod == "GET" {
		err := mw.HandleGETRequest(req)
		if err != nil {
			return err
		}
	}
	// if here, all should be well or nothing was actually done,
	// either way return happily
	return nil
}

// HandleGETRequest handles get request
func (mw *XssMw) HandleGETRequest(req *request.Request) error {
	p := mw.GetBlueMondayPolicy()
	queryParams := req.Query
	var fieldToSkip = map[string]bool{}
	for _, fts := range mw.FieldsToSkip {
		fieldToSkip[fts] = true
	}
	for key, value := range queryParams {
		if fieldToSkip[key] {
			continue
		}
		queryParams.Del(key)
		queryParams.Set(key, p.Sanitize(value))
	}
	return nil
}

// http://127.0.0.1:8080/?user=%3Ca%20onblur=%22alert(secret)%22%20href=%22http://www.google.com%22%3EGoogle%3C/a%3E
