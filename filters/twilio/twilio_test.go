package filter

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTwilio(t *testing.T) {
	t.Parallel()

	twloResp := &Response{}
	buf := bytes.NewBuffer(response())
	if err := json.NewDecoder(buf).Decode(twloResp); err != nil {
		t.Fatal(err)
	}

	results := twloResp.AddOnsResults()

	if len(results) != 1 {
		t.Fatalf("exp: 1, got: %d", len(results))
	}

	if results[0].SpamConfidence() != 1.0 {
		t.Fatalf("exp: 1.0, got %f", results[0].SpamConfidence())
	}
}

func response() []byte {
	return []byte(`{
		"caller_name": null,
		"country_code": "US",
		"phone_number": "+12022831710",
		"national_format": "(202) 283-1710",
		"carrier": null,
		"add_ons": {
			"status": "successful",
			"message": null,
			"code": null,
			"results": {
				"whitepages_pro_phone_rep": {
					"request_sid": "XR1234567890THISISABOGUSSIDAAAAAAA",
					"status": "successful",
					"message": null,
					"code": null,
					"result": {
						"id": "Phone.abcdefef-a1bc-a45b-fed6-abcd1234ef56.Durable",
						"phone_number": "2022831710",
						"reputation_level": 1,
						"reputation_details": {
							"score": 1,
							"type": "UncertainType",
							"category": null
						},
						"volume_score": 1,
						"report_count": 4,
						"error": null,
						"warnings": [ ]
					}
				}
			}
		},
		"url": "https://lookups.twilio.com/v1/PhoneNumbers/+12022831710"
	}`)
}
