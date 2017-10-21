package whitepagespro

import "fmt"

// Example resoponse JSON for one of the IRS' phone numbers.
// (SID and result id have been changed to bogus values.)

//{
//	caller_name: null,
//	country_code: "US",
//	phone_number: "+12022831710",
//	national_format: "(202) 283-1710",
//	carrier: null,
//	add_ons: {
//		status: "successful",
//		message: null,
//		code: null,
//		results: {
//			whitepages_pro_phone_rep: {
//				request_sid: "XR1234567890THISISABOGUSSIDAAAAAAA",
//				status: "successful",
//				message: null,
//				code: null,
//				result: {
//					id: "Phone.abcdefef-a1bc-a45b-fed6-abcd1234ef56.Durable",
//					phone_number: "2022831710",
//					reputation_level: 1,
//					reputation_details: {
//						score: 1,
//						type: "UncertainType",
//						category: null
//					},
//					volume_score: 1,
//					report_count: 4,
//					error: null,
//					warnings: [ ]
//				}
//			}
//		}
//	},
//	url: "https://lookups.twilio.com/v1/PhoneNumbers/+12022831710"
//}

type Response struct {
	RequestSID string `json:"request_sid"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Code       string `json:"code"`
	Result     Result `json:"result"`
}

type Result struct {
	ID                string            `json:"id"`
	PhoneNumber       string            `json:"phone_number"`
	ReputationLevel   int               `json:"reputation_level"`
	VolumeScore       int               `json:"volume_score"`
	ReportCount       int               `json:"report_count"`
	Error             *ResultError      `json:"error"`
	Warnings          []string          `json:"warnings"`
	ReputationDetails ReputationDetails `json:"reputation_details"`
}

type ReputationDetails struct {
	Score    int    `json:"score"`
	Type     string `json:"type"`
	Category string `json:"category"`
}

type ResultError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (r *Response) Name() string {
	return "White Pages Pro Phone Reputation"
}

func (r *Response) Error() error {
	if r.Result.Error == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", r.Result.Error.Name, r.Result.Error.Message)
}

func (r *Response) SpamConfidence() float64 {
	return float64(r.Result.ReputationDetails.Score)
}

func (r *Response) SpamDescription() string {
	rd := r.Result.ReputationDetails
	return fmt.Sprintf("%s: %s", rd.Type, rd.Category)
}
