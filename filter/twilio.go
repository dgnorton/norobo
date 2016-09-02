package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/dgnorton/norobo"
)

type Twilio struct {
	sid   string
	token string
}

func NewTwilio(sid, token string) *Twilio {
	return &Twilio{
		sid:   sid,
		token: token,
	}
}

func (t *Twilio) Check(c *norobo.Call, result chan *norobo.FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()

		// Build the HTTP request to Twilio's Lookup service.
		url := fmt.Sprintf("https://lookups.twilio.com/v1/PhoneNumbers/%s?AddOns=", c.Number)
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

		// Check Twilio response.
		category, err := twloResp.AddOns.WppRepCategory()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		} else if category != "NotSpam" && category != "NotAplicable" {
			result <- &norobo.FilterResult{Match: true, Action: t.Action(), Filter: t, Description: "Twilio"}
			return
		}

		result <- &norobo.FilterResult{Action: norobo.Allow}
	}()
}

func (f *Twilio) Action() norobo.Action { return norobo.Block }
func (f *Twilio) Description() string   { return "Twilio" }

//{
//    "add_ons": {
//        "code": null,
//        "message": null,
//        "results": {},
//        "status": "successful"
//    },
//    "caller_name": null,
//    "carrier": null,
//    "country_code": "US",
//    "national_format": "(727) 810-3953",
//    "phone_number": "+17278103953",
//    "url": "https://lookups.twilio.com/v1/PhoneNumbers/+17278103953"
//}

//{
//    "add_ons": {
//        "code": null,
//        "message": null,
//        "results": {
//            "whitepages_pro_phone_rep": {
//                "code": null,
//                "message": null,
//                "request_sid": "XR8459df6a1f91ca92f738f11fac829f97",
//                "result": {
//                    "messages": [],
//                    "results": [
//                        {
//                            "phone_number": "7278103953",
//                            "reputation": {
//                                "details": [
//                                    {
//                                        "category": "NotApplicable",
//                                        "score": 2,
//                                        "type": "NotApplicable"
//                                    }
//                                ],
//                                "level": 1,
//                                "report_count": 10,
//                                "volume_score": 1
//                            }
//                        }
//                    ]
//                },
//                "status": "successful"
//            }
//        },
//        "status": "successful"
//    },
//    "caller_name": null,
//    "carrier": null,
//    "country_code": "US",
//    "national_format": "(727) 810-3953",
//    "phone_number": "+17278103953",
//    "url": "https://lookups.twilio.com/v1/PhoneNumbers/+17278103953"
//}

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
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Status  string      `json:"status"`
	Results interface{} `json:"results"`
}

// WppRepNumber returns the witepages_pro_phone_rep phone_number.
func (r *AddOnsResponse) WppRepNumber() (string, error) {
	results, ok := r.Results.(map[string]interface{})
	if !ok {
		return "", errors.New("no add-on results")
	}

	wppprI, ok := results["whitepages_pro_phone_rep"]
	if !ok {
		return "", errors.New("no results for whitepages_pro_phone_rep add-on")
	}
	wpppr := wppprI.(map[string]interface{})

	status := wpppr["status"].(string)
	if status != "successful" {
		return "", fmt.Errorf("witepages_pro_phone_rep add-on status: %s", status)
	}

	wppprResults := wpppr["result"].(map[string]interface{})["results"].([]interface{})

	return wppprResults[0].(map[string]interface{})["phone_number"].(string), nil
}

// WppRepCategory returns the witepages_pro_phone_rep category.
func (r *AddOnsResponse) WppRepCategory() (string, error) {
	results, ok := r.Results.(map[string]interface{})
	if !ok {
		return "", errors.New("no add-on results")
	}

	wppprI, ok := results["whitepages_pro_phone_rep"]
	if !ok {
		return "", errors.New("no results for whitepages_pro_phone_rep add-on")
	}
	wpppr := wppprI.(map[string]interface{})

	status := wpppr["status"].(string)
	if status != "successful" {
		return "", fmt.Errorf("witepages_pro_phone_rep add-on status: %s", status)
	}

	wppprResults := wpppr["result"].(map[string]interface{})["results"].([]interface{})
	reputation := wppprResults[0].(map[string]interface{})["reputation"].(map[string]interface{})
	details := reputation["details"].([]interface{})

	return details[0].(map[string]interface{})["category"].(string), nil
}
