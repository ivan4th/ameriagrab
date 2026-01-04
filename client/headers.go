package client

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

const (
	UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0"

	AuthBaseURL  = "https://account.myameria.am"
	APIBaseURL   = "https://ob.myameria.am"
	RedirectURI  = "https://myameria.am/"
	OAuthClient  = "banqr-online"
	ClientSecret = "b54f3f83-a696-48da-95de-b9b4154a3944"

	PollInterval = 3 * time.Second
	PollTimeout  = 120 * time.Second
)

// AddAPIHeaders adds common headers for API requests
func (c *Client) AddAPIHeaders(req *http.Request, accessToken string) {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Referer", RedirectURI)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Banqr-2FA", BuildTwoFAHeader())
	req.Header.Set("X-Banqr-CDDC", BuildCDDCHeader())
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Timezone-Offset", "-240")
	req.Header.Set("Client-Time", time.Now().Format("15:04:05"))
	req.Header.Set("Client-Id", c.ClientID)
	req.Header.Set("Locale", "ru")
	req.Header.Set("Origin", RedirectURI[:len(RedirectURI)-1])
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Priority", "u=0")
}

// AddFirefoxHeaders adds Firefox browser headers for auth requests
func (c *Client) AddFirefoxHeaders(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Connection", "keep-alive")
}

// BuildCDDCHeader builds the X-Banqr-CDDC header
func BuildCDDCHeader() string {
	cddc := map[string]interface{}{
		"browserCDDC": map[string]interface{}{
			"fingerprintRaw":  `{"browser":{"userAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:146.0) Gecko/20100101 Firefox/146.0","applicationVersion":"5.0 (Macintosh)","applicationCode":"Mozilla","applicationName":"Netscape","cookieEnabled":true,"javaEnabled":false},"support":{"ajax":true,"boxModel":true,"changeBubbles":true,"checkClone":true,"checkOn":true,"cors":true,"cssFloat":true,"hrefNormalized":true,"htmlSerialize":true,"leadingWhitespace":true,"noCloneChecked":true,"noCloneEvent":true,"opacity":true,"optDisabled":true,"style":true,"submitBubbles":true,"tbody":true},"device":{"screenWidth":1920,"screenHeight":1080,"os":"Apple MacOS","language":"en-US","platform":"MacIntel","timeZone":-240},"plugin":[{"name":"PDF Viewer","file":"internal-pdf-viewer"},{"name":"Chrome PDF Viewer","file":"internal-pdf-viewer"},{"name":"Chromium PDF Viewer","file":"internal-pdf-viewer"},{"name":"Microsoft Edge PDF Viewer","file":"internal-pdf-viewer"},{"name":"WebKit built-in PDF","file":"internal-pdf-viewer"}]}`,
			"fingerprintHash": "2ce4831e26386fd68e0554378fc85ef41902ad5f6ab0ba61a74265ebb05923b0",
		},
	}
	jsonBytes, _ := json.Marshal(cddc)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

// BuildTwoFAHeader builds the X-Banqr-2FA header
func BuildTwoFAHeader() string {
	twoFA := map[string]interface{}{
		"otpEvaluation": map[string]interface{}{
			"securityProviderName": "ONESPAN-PHONE",
		},
	}
	jsonBytes, _ := json.Marshal(twoFA)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}
