package slackgoround

import (
	"appengine"
        "appengine/urlfetch"
	"bytes"
	"encoding/xml"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
)

func init() {
	http.HandleFunc("/", handler)
}

type Prediction struct {
        Seconds int `xml:"seconds,attr"`
        Vehicle string `xml:"vehicle,attr"`
}

type Direction struct {
        Title string `xml:"title,attr"`
        Predictions []Prediction `xml:"prediction"`
}

type Predictions struct {
        Agency string `xml:"agencyTitle,attr"`
        Route string `xml:"routeTitle,attr"`
        Stop string `xml:"stopTitle,attr"`
        Directions []Direction `xml:"direction"`
}

type Response struct {
	Predictions Predictions `xml:"predictions"`
}

var badAmpFinder *regexp.Regexp = regexp.MustCompile("&[^ ;]* ")

// NextBus tends to be terrible and return non-entity ampersands.
// This function replaces any ampersand that looks like it's not
// an entity with &amp;.  This is done by finding ampersands that
// have a space before a semicolon.
// Note that this function could be considerably more efficient
// but this is how I thought to do it first and I don't want to 
// reimplement it.  Ideally, it'd be a replacement io.Reader that
// would buffer data after seeing a & and, failing to see a
// semicolon before seeing another space, would return the data
// it read into that buffer after emitting a &amp;.  This could
// be made a bit less dangerous by only reading up to the maximum
// length that an entity can be.
// For another day.  Probably not though.  This is plenty fast
// enough :D

func fixXMLAmps(badXML []byte) []byte {
	badAmps := badAmpFinder.FindAllIndex([]byte(badXML), -1)

	fixedXMLBuffer := &bytes.Buffer{}
	badXMLBuffer := bytes.NewReader([]byte(badXML))

	readSoFar := 0
	for _, elt := range badAmps {
		io.CopyN(fixedXMLBuffer, badXMLBuffer, int64(elt[0] - readSoFar))
		fixedXMLBuffer.Write([]byte("&amp;"))
		badXMLBuffer.ReadByte()
		readSoFar += elt[0] + 1
	}
	io.Copy(fixedXMLBuffer, badXMLBuffer)
	return fixedXMLBuffer.Bytes()
}

var stopMatcher *regexp.Regexp = regexp.MustCompile(".* ([0-9]+)[ \t]*$")

func handler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
        client := urlfetch.Client(c)

	err := r.ParseForm()

	if err != nil {
		fmt.Fprintf(w, `{"text": "Error: Error parsing form data: %s", username: "Emery Go Round"}`, err.Error())
		return
	}

	inputStop := "5319"
	stopMatches := stopMatcher.FindStringSubmatch(r.Form.Get("text"))
	if len(stopMatches) > 0 {
		inputStop = stopMatches[1]
	}

	nextBusRequest := fmt.Sprintf("http://webservices.nextbus.com/service/publicXMLFeed?command=predictions&a=emery&stopId=%s", inputStop)

        resp, err := client.Get(nextBusRequest)
        //resp, err := client.Get("http://webservices.nextbus.com/service/publicXMLFeed?command=predictions&a=sf-muni&stopId=17205&r=BUS")
	
	predictionResponse := Response{}
        rawXML, err := ioutil.ReadAll(resp.Body)
        if err != nil {
		fmt.Fprintf(w, `{"text": "Error: Couldn't read response from nextbus: %s", username: "Emery Go Round"}`, err.Error())
		return
        }

        err = xml.Unmarshal(fixXMLAmps(rawXML), &predictionResponse)
	if err != nil {
		fmt.Fprintf(w, `{"text": "Error: Couldn't unmarshal XML from nextbus: %s", username: "Emery Go Round"}`, err.Error())
		return
        }
	prediction := predictionResponse.Predictions
	agency := prediction.Agency
	if len(prediction.Directions) < 1 || len(prediction.Directions[0].Predictions) < 1 {
		fmt.Fprintf(w, `{"text": "No predictions available", username: "Emery Go Round"}`);
		return
	}
	firstDirection := prediction.Directions[0]
	firstPrediction := firstDirection.Predictions[0]
	bus := firstPrediction.Vehicle
	direction := firstDirection.Title
	stop := prediction.Stop
	seconds := firstPrediction.Seconds
	duration := time.Duration(time.Duration(seconds) * time.Second).String()
	textResponse := fmt.Sprintf(`%s bus #%s going %s will arrive at %s in %s`, agency, bus, direction, stop, duration)
	textToPrint, err := json.Marshal(textResponse)
	if err != nil {
		fmt.Fprintf(w, `{"text": "Error: Couldn't marshal the response... weird.  %s", username: "Emery Go Round"}`, err.Error())
		return
	}

	fmt.Fprintf(w, `{"text": %s, "username": "Emery Go Round"}`, textToPrint)

	// TODO: Let's make it work with "stops" and with a stop ID.
}
