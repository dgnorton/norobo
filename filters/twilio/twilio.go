package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/dgnorton/norobo"
	"github.com/dgnorton/norobo/filters/twilio/addons"
	"github.com/dgnorton/norobo/filters/twilio/addons/whitepagespro"
)

type Twilio struct {
	sid               string
	token             string
	minSpamConfidence float64
}

func NewTwilio(sid, token string) *Twilio {
	return &Twilio{
		sid:               sid,
		token:             token,
		minSpamConfidence: 40.0,
	}
}

func (t *Twilio) Check(c *norobo.Call, result chan *norobo.FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()

		// Build the HTTP request to Twilio's Lookup service.
		url := fmt.Sprintf("https://lookups.twilio.com/v1/PhoneNumbers/%s?AddOns=whitepages_pro_phone_rep", c.Number)
		fmt.Println(url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}
		req.SetBasicAuth(t.sid, t.token)

		// Make the request.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}
		defer resp.Body.Close()

		// Decode the JSON response.
		twloResp := &Response{}
		if err := json.NewDecoder(resp.Body).Decode(twloResp); err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		// Get results from Twilio addons.
		results := twloResp.AddOnsResults()

		if len(results) == 0 {
			result <- &norobo.FilterResult{Err: errors.New("no response from Twilio Whitepages Pro Phone Rep"), Action: norobo.Allow}
			return
		}

		// See if any of the addons think this caller is spam.
		for _, r := range results {
			if r.SpamConfidence() >= t.minSpamConfidence {
				result <- &norobo.FilterResult{Match: true, Action: t.Action(), Filter: t, Description: r.SpamDescription()}
				return
			}
		}

		// Addons think this caller is legitimate.
		result <- &norobo.FilterResult{Action: norobo.Allow}
	}()
}

func (f *Twilio) Action() norobo.Action { return norobo.Block }
func (f *Twilio) Description() string   { return "Twilio (White Pages Pro Phone Reputation)" }

type Response struct {
	CallerName     string          `json:"caller_name"`
	Carrier        string          `json:"carrier"`
	CountryCode    string          `json:"country_code"`
	NationalFormat string          `json:"national_format"`
	PhoneNumber    string          `json:"phone_number"`
	URL            string          `json:"url"`
	AddOns         *AddOnsResponse `json:"add_ons"`
}

type AddOnsResponse struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Status  string  `json:"status"`
	Results Results `json:"results"`
}

type Results struct {
	WhitePagesProPhoneRep *whitepagespro.Response `json:"whitepages_pro_phone_rep"`
}

func (r *Response) AddOnsResults() []addon.Result {
	results := []addon.Result{}

	if r.AddOns == nil {
		return results
	}

	ar := r.AddOns.Results
	if ar.WhitePagesProPhoneRep != nil {
		results = append(results, ar.WhitePagesProPhoneRep)
	}
	return results
}
