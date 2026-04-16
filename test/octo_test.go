package test

import (
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v3/client"
)

func TestGetOcto(t *testing.T) {
	someid := "205051"
	fromv := "0"

	resp, err := testConfig.HttpClient.Get(fmt.Sprintf("%s/v2/pub/a/212/v/%s/list/%s", testConfig.HttpUrl, someid, fromv), client.Config{
		Header: map[string]string{
			"Accept":     "application/x-protobuf,x-octo-app/212",
			"X-OCTO-KEY": "xop8c3tqrl8dcgij6mc9y4yf5o7btknt",
		},
	})
	if err != nil {
		t.Fatalf("get octo manifest error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("get octo manifest error, bad status code: %d", resp.StatusCode())
	}
}

func TestGetOctoAsset(t *testing.T) {
	resp1, err := testConfig.HttpClient.Get(fmt.Sprintf("%s/solis-202050-assetbundle/pN7hs9", testConfig.HttpUrl), client.Config{
		Param: map[string]string{
			"generation": "1711343144215786",
			"alt":        "media",
		},
	})
	if err != nil {
		t.Fatalf("failed getting octo asset: %v", err)
	}
	if resp1.StatusCode() != 200 {
		t.Fatalf("failed getting octo asset: abnormal status %s", resp1.Status())
	}
	if len(resp1.Body()) == 0 {
		t.Fatalf("failed getting octo asset: empty body")
	}

	resp2, err := testConfig.HttpClient.Get(fmt.Sprintf("%s/solis-202050-resources/e44Syl", testConfig.HttpUrl), client.Config{
		Param: map[string]string{
			"generation": "1711349430629139",
			"alt":        "media",
		},
	})
	if err != nil {
		t.Fatalf("failed getting octo asset: %v", err)
	}
	if resp2.StatusCode() != 200 {
		t.Fatalf("failed getting octo asset: abnormal status %s", resp2.Status())
	}
	if len(resp2.Body()) == 0 {
		t.Fatalf("failed getting octo asset: empty body")
	}
}
