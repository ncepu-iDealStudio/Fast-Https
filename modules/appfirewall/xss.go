package appfirewall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fast-https/modules/core/request"
	"fast-https/utils/logger"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// Config struct for passing options
type XssMw struct {
	// List of fields to not filter. i.e. password, created_on, created_at, etc
	// password is set to skip by the system i.e. FieldsToSkip = []string{"password", "cre_date"}
	FieldsToSkip []string

	// Bluemonday comes with two default policies
	// Two options StrictPolicy // the default
	//             UGCPolicy
	// or you can specify you own policy
	// define it somewhere in your package so that you can call it here
	// see https://github.com/microcosm-cc/bluemonday/blob/master/policies.go
	BmPolicy string
}

type XssMwJson map[string]interface{}

// Get which Bluemonday policy
func (mw *XssMw) GetBlueMondayPolicy() *bluemonday.Policy {

	if mw.BmPolicy == "UGCPolicy" {
		return bluemonday.UGCPolicy()
		//} else if mw.BmPolicy == "New" {
	}

	// default
	return bluemonday.StrictPolicy()
}

func HandleXss(req *request.Request) bool {
	logger.Debug("This is appfirewall, handle sql")
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
			err := mw.HandleJson(req)
			if err != nil {
				return err
			}
		} else if ctHdr == "application/x-www-form-urlencoded" {
			err := mw.HandleXFormEncoded(req)
			if err != nil {
				return err
			}
		} else if strings.Contains(ctHdr, "multipart/form-data") {
			err := mw.HandleMultiPartFormData(req, ctHdr)
			if err != nil {
				return err
			}
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

func (mw *XssMw) HandleXFormEncoded(req *request.Request) error {
	// if req.BodyLen == 0 {
	// 	return nil
	// }

	// https://golang.org/src/net/http/httputil/dump.go
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(&req.Body); err != nil {
		return err
	}

	m, uerr := url.ParseQuery(buf.String())
	if uerr != nil {
		return uerr
	}

	p := mw.GetBlueMondayPolicy()

	var bq bytes.Buffer
	for k, v := range m {
		//fmt.Println(k, " => ", v)
		bq.WriteString(k)
		bq.WriteByte('=')

		// do fields to skip
		var fndFld bool = false
		for _, fts := range mw.FieldsToSkip {
			if k == fts {
				// dont saniitize these fields
				bq.WriteString(url.QueryEscape(v[0]))
				fndFld = true
				break
			}
		}
		if !fndFld {
			bq.WriteString(url.QueryEscape(p.Sanitize(v[0])))
		}
		bq.WriteByte('&')
	}

	if bq.Len() > 1 {
		bq.Truncate(bq.Len() - 1) // remove last '&'
		bodOut := bq.String()
		req.Body = *bytes.NewBuffer([]byte(bodOut))
	} else {
		req.Body = *bytes.NewBuffer(buf.Bytes())
	}

	return nil
}

func (mw *XssMw) HandleMultiPartFormData(req *request.Request, ctHdr string) error {
	var ioreader io.Reader = &req.Body

	boundary := ctHdr[strings.Index(ctHdr, "boundary=")+9:]

	reader := multipart.NewReader(ioreader, boundary)

	var multiPrtFrm bytes.Buffer
	// unknown, so make up some param limit - 100 max should be enough
	for i := 0; i < 100; i++ {
		part, err := reader.NextPart()
		if err != nil {
			//fmt.Println("didn't get a part")
			break
		}

		var buf bytes.Buffer
		n, err := io.Copy(&buf, part)
		if err != nil {
			return err
		}
		if n <= 0 {
			return errors.New("error recreating Multipart form Request")
		}
		// https://golang.org/src/mime/multipart/multipart_test.go line 230
		multiPrtFrm.WriteString(`--` + boundary + "\r\n")
		// dont sanitize file content
		if part.FileName() != "" {
			fn := part.FileName()
			mtype := part.Header.Get("Content-Type")
			multiPrtFrm.WriteString(`Content-Disposition: form-data; name="` + part.FormName() + "\"; ")
			multiPrtFrm.WriteString(`filename="` + fn + "\";\r\n")
			// default to application/octet-stream
			if mtype == "" {
				mtype = `application/octet-stream`
			}
			multiPrtFrm.WriteString(`Content-Type: ` + mtype + "\r\n\r\n")
			multiPrtFrm.WriteString(buf.String() + "\r\n")
		} else {
			multiPrtFrm.WriteString(`Content-Disposition: form-data; name="` + part.FormName() + "\";\r\n\r\n")
			p := bluemonday.StrictPolicy()
			if part.FormName() == "password" {
				multiPrtFrm.WriteString(buf.String() + "\r\n")
			} else {
				multiPrtFrm.WriteString(p.Sanitize(buf.String()) + "\r\n")
			}
		}
	}
	multiPrtFrm.WriteString("--" + boundary + "--\r\n")

	req.Body = *bytes.NewBuffer(multiPrtFrm.Bytes())

	return nil
}

func (mw *XssMw) HandleJson(req *request.Request) error {
	jsonBod, err := decodeJson(&req.Body)
	if err != nil {
		return err
	}

	buff, err := mw.jsonToStringMap(bytes.Buffer{}, jsonBod)
	if err != nil {
		return err
	}
	err = mw.SetRequestBodyJson(req, buff)
	if err != nil {
		//fmt.Println("Set request body failed")
		return errors.New("set request.body error")
	}
	return nil
}

func decodeJson(content io.Reader) (interface{}, error) {
	var jsonBod interface{}
	d := json.NewDecoder(content)
	d.UseNumber()
	err := d.Decode(&jsonBod)
	if err != nil {
		return nil, err
	}
	return jsonBod, err
}

func (mw *XssMw) jsonToStringMap(buff bytes.Buffer, jsonBod interface{}) (bytes.Buffer, error) {
	switch jbt := jsonBod.(type) {
	case map[string]interface{}:
		xmj := jsonBod.(map[string]interface{})
		var sbuff bytes.Buffer
		buff := mw.ConstructJson(xmj, sbuff)
		return buff, nil
	// TODO: need a test to prove this
	case []interface{}:
		var multiRec bytes.Buffer
		multiRec.WriteByte('[')
		for _, n := range jbt {
			xmj := n.(map[string]interface{})
			var sbuff bytes.Buffer
			buff = mw.ConstructJson(xmj, sbuff)
			multiRec.WriteString(buff.String())
			multiRec.WriteByte(',')
		}
		multiRec.Truncate(multiRec.Len() - 1) // remove last ','
		multiRec.WriteByte(']')
		return multiRec, nil
	default:
		return bytes.Buffer{}, errors.New("unknown content type received")
	}
}

// encode processed body back to json and re-set http request body
func (mw *XssMw) SetRequestBodyJson(req *request.Request, buff bytes.Buffer) error {
	// XXX clean up - probably don't need to convert to string
	// only to convert back to NewBuffer for NopCloser
	bodOut := buff.String()

	enc := json.NewEncoder(io.Discard)
	if merr := enc.Encode(&bodOut); merr != nil {
		return merr
	}
	req.Body = *bytes.NewBuffer([]byte(bodOut))

	return nil
}

// De-constructs the http request body
// removes undesirable content
// keeps the good content to construct
// returns the cleaned http request
// Map to Bytes (struct to json string...)
func (mw *XssMw) ConstructJson(xmj XssMwJson, buff bytes.Buffer) bytes.Buffer {
	//var buff bytes.Buffer
	buff.WriteByte('{')

	p := mw.GetBlueMondayPolicy()

	m := xmj
	for k, v := range m {
		buff.WriteByte('"')
		buff.WriteString(k)
		buff.WriteByte('"')
		buff.WriteByte(':')

		// do fields to skip
		var fndFld bool = false
		for _, fts := range mw.FieldsToSkip {
			if string(k) == fts {
				buff.WriteString(fmt.Sprintf("%q", v))
				buff.WriteByte(',')
				fndFld = true
				break
			}
		}
		if fndFld {
			continue
		}

		var b bytes.Buffer
		apndBuff := mw.buildJsonApplyPolicy(v, b, p)
		buff.WriteString(apndBuff.String())
	}
	buff.Truncate(buff.Len() - 1) // remove last ','
	buff.WriteByte('}')

	return buff
}

func (mw *XssMw) buildJsonApplyPolicy(interf interface{}, buff bytes.Buffer, p *bluemonday.Policy) bytes.Buffer {
	switch v := interf.(type) {
	case map[string]interface{}:
		var sbuff bytes.Buffer
		scnd := mw.ConstructJson(v, sbuff)
		buff.WriteString(scnd.String())
		buff.WriteByte(',')
	case []interface{}:
		b := mw.unravelSlice(v, p)
		buff.WriteString(b.String())
		buff.WriteByte(',')
	case json.Number:
		buff.WriteString(p.Sanitize(fmt.Sprintf("%v", v)))
		buff.WriteByte(',')
	case string:
		buff.WriteString(fmt.Sprintf("%q", p.Sanitize(v)))
		buff.WriteByte(',')
	case float64:
		buff.WriteString(p.Sanitize(strconv.FormatFloat(v, 'g', 0, 64)))
		buff.WriteByte(',')
	default:
		if v == nil {
			buff.WriteString("null")
			buff.WriteByte(',')
		} else {
			buff.WriteString(p.Sanitize(fmt.Sprintf("%v", v)))
			buff.WriteByte(',')
		}
	}
	return buff
}

func (mw *XssMw) unravelSlice(slce []interface{}, p *bluemonday.Policy) bytes.Buffer {
	var buff bytes.Buffer
	buff.WriteByte('[')
	for _, n := range slce {
		switch nn := n.(type) {
		case map[string]interface{}:
			var sbuff bytes.Buffer
			scnd := mw.ConstructJson(nn, sbuff)
			buff.WriteString(scnd.String())
			buff.WriteByte(',')
		case string:
			buff.WriteString(fmt.Sprintf("%q", p.Sanitize(nn)))
			buff.WriteByte(',')
		}
	}
	buff.Truncate(buff.Len() - 1) // remove last ','
	buff.WriteByte(']')
	return buff
}
