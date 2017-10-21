package filter

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTwilio_WppRepNumber(t *testing.T) {
	str := `{
    "add_ons": {
        "code": null,
        "message": null,
        "results": {
            "whitepages_pro_phone_rep": {
                "code": null,
                "message": null,
                "request_sid": "XR8459df6a1f91ca92f738f11fac829f97",
                "result": {
                    "messages": [],
                    "results": [
                        {
                            "phone_number": "7278103953",
                            "reputation": {
                                "details": [
                                    {
                                        "category": "NotApplicable",
                                        "score": 2,
                                        "type": "NotApplicable"
                                    }
                                ],
                                "level": 1,
                                "report_count": 10,
                                "volume_score": 1
                            }
                        }
                    ]
                },
                "status": "successful"
            }
        },
        "status": "successful"
    },
    "caller_name": null,
    "carrier": null,
    "country_code": "US",
    "national_format": "(727) 810-3953",
    "phone_number": "+17278103953",
    "url": "https://lookups.twilio.com/v1/PhoneNumbers/+17278103953"
}`

	twloResp := &Response{}
	buf := bytes.NewBuffer([]byte(str))
	if err := json.NewDecoder(buf).Decode(twloResp); err != nil {
		t.Fatal(err)
	}

	if number, err := twloResp.AddOns.WppRepNumber(); err != nil {
		t.Fatal(err)
	} else if number != "7278103953" {
		t.Fatalf("unexpected number: %s", number)
	}
}

func TestTwilio_WppRepCategory(t *testing.T) {
	str := `{
    "add_ons": {
        "code": null,
        "message": null,
        "results": {
            "whitepages_pro_phone_rep": {
                "code": null,
                "message": null,
                "request_sid": "XR8459df6a1f91ca92f738f11fac829f97",
                "result": {
                    "messages": [],
                    "results": [
                        {
                            "phone_number": "7278103953",
                            "reputation": {
                                "details": [
                                    {
                                        "category": "NotApplicable",
                                        "score": 2,
                                        "type": "NotApplicable"
                                    }
                                ],
                                "level": 1,
                                "report_count": 10,
                                "volume_score": 1
                            }
                        }
                    ]
                },
                "status": "successful"
            }
        },
        "status": "successful"
    },
    "caller_name": null,
    "carrier": null,
    "country_code": "US",
    "national_format": "(727) 810-3953",
    "phone_number": "+17278103953",
    "url": "https://lookups.twilio.com/v1/PhoneNumbers/+17278103953"
}`

	twloResp := &Response{}
	buf := bytes.NewBuffer([]byte(str))
	if err := json.NewDecoder(buf).Decode(twloResp); err != nil {
		t.Fatal(err)
	}

	if number, err := twloResp.AddOns.WppRepCategory(); err != nil {
		t.Fatal(err)
	} else if number != "NotApplicable" {
		t.Fatalf("unexpected number: %s", number)
	}
}
