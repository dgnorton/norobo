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

		// Check Twilio response.
		if twloResp.AddOns == nil {
			result <- &norobo.FilterResult{Err: errors.New("no response from Twilio Whitepages Pro Phone Rep"), Action: norobo.Allow}
			return
		}

		category, err := twloResp.AddOns.WppRepCategory()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		typ, err := twloResp.AddOns.WppRepType()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		if category != "NotSpam" && category != "NotApplicable" {
			desc := fmt.Sprintf("%s:%s", typ, category)
			result <- &norobo.FilterResult{Match: true, Action: t.Action(), Filter: t, Description: desc}
			return
		}

		result <- &norobo.FilterResult{Action: norobo.Allow}
	}()
}

func (f *Twilio) Action() norobo.Action { return norobo.Block }
func (f *Twilio) Description() string   { return "Twilio (White Pages Pro Phone Reputation)" }

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
	if err := r.whitePagesProPhoneRepStatus(); err != nil {
		return "", err
	}

	results, err := r.whitePagesProPhoneRepResults()
	if err != nil {
		return "", err
	}

	return results[0].(map[string]interface{})["phone_number"].(string), nil
}

// WppRepType returns the witepages_pro_phone_rep type.
func (r *AddOnsResponse) WppRepType() (string, error) {
	if err := r.whitePagesProPhoneRepStatus(); err != nil {
		return "", err
	}

	details, err := r.whitePagesProPhoneRepDetails()
	if err != nil {
		return "", err
	}

	return details[0].(map[string]interface{})["type"].(string), nil
}

// WppRepCategory returns the witepages_pro_phone_rep category.
func (r *AddOnsResponse) WppRepCategory() (string, error) {
	if err := r.whitePagesProPhoneRepStatus(); err != nil {
		return "", err
	}

	details, err := r.whitePagesProPhoneRepDetails()
	if err != nil {
		return "", err
	}

	return details[0].(map[string]interface{})["category"].(string), nil
}

func (r *AddOnsResponse) addOnResults() (map[string]interface{}, error) {
	results, ok := r.Results.(map[string]interface{})
	if !ok {
		return nil, errors.New("no add-on results")
	}
	return results, nil
}

func (r *AddOnsResponse) whitePagesProPhoneRep() (map[string]interface{}, error) {
	results, err := r.addOnResults()
	if err != nil {
		return nil, err
	}

	wppprI, ok := results["whitepages_pro_phone_rep"]
	if !ok {
		return nil, errors.New("no results for whitepages_pro_phone_rep add-on")
	}

	return wppprI.(map[string]interface{}), nil
}

func (r *AddOnsResponse) whitePagesProPhoneRepStatus() error {
	wpppr, err := r.whitePagesProPhoneRep()
	if err != nil {
		return err
	}

	status := wpppr["status"].(string)
	if status != "successful" {
		return fmt.Errorf("witepages_pro_phone_rep add-on status: %s", status)
	}

	return nil
}

func (r *AddOnsResponse) whitePagesProPhoneRepResults() ([]interface{}, error) {
	wpppr, err := r.whitePagesProPhoneRep()
	if err != nil {
		return nil, err
	}

	return wpppr["result"].(map[string]interface{})["results"].([]interface{}), nil
}

func (r *AddOnsResponse) whitePagesProPhoneRepReputation() (map[string]interface{}, error) {
	results, err := r.whitePagesProPhoneRepResults()
	if err != nil {
		return nil, err
	}

	return results[0].(map[string]interface{})["reputation"].(map[string]interface{}), nil
}

func (r *AddOnsResponse) whitePagesProPhoneRepDetails() ([]interface{}, error) {
	reputation, err := r.whitePagesProPhoneRepReputation()
	if err != nil {
		return nil, err
	}

	return reputation["details"].([]interface{}), nil
}
