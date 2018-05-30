// Bazaar is an wrapper for cafebazaar.ir purchase API
package cafebazaar

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/dena-a/inappbilling/inappbillingerror"

	"github.com/dena-a/inappbilling/jsonconfig"
)

// type Cafebazaar struct {
// 	configuration configuration `json:"cafebazaar"`
// }

type configuration struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	ExpiresAt    int64  `json:"expires_at"`
}

type token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

type Client struct {
	config configuration
	mu     sync.Mutex
}

// Create a new client and define it's configuration based on given argumans
func NewClient(clientId string, clientSecret string, refreshToken string) (c Client, err error) {
	c = Client{}

	c.config = configuration{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
	}

	err = c.RefreshToken()
	return c, err
}

var config = &configuration{}

func (c *configuration) ParseJSON(b []byte) error {
	return json.Unmarshal(b, &c)
}

// Create a new client based on config file
func NewClientFromFile(path string) (c Client, err error) {
	c = Client{}
	jsonconfig.Load(path, config)
	c.config = *config
	if c.config.ExpiresAt > time.Now().Unix() {
		return c, err
	}

	err = c.RefreshToken()

	configJson, _ := json.Marshal(c.config)
	err = ioutil.WriteFile(path, configJson, 0644)

	return c, err
}

// Refresh access token using user credentials
func (c *Client) RefreshToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	endpoint := NewEndpoint("auth")
	url := endpoint.generate()

	// Create refresh token form based with user credentials
	form := Form{
		"grant_type":    "refresh_token",
		"client_id":     c.config.ClientId,
		"client_secret": c.config.ClientSecret,
		"refresh_token": c.config.RefreshToken,
	}

	// Create request body and gather content type
	body, contentType := form.Build()

	// Initiate a new request with POST method
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return errors.New("CannotInitiateRequest")
	}

	// Set content type of the request
	req.Header.Add("Content-Type", contentType)

	// Initiate a HTTP client for sending request
	client := &http.Client{}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("CannotSendRequest")
	}

	// Close response body before function exit
	defer resp.Body.Close()

	// Check for response body
	if resp.StatusCode != http.StatusOK {
		return errors.New("InvalidCredentials")
	}

	// Decode request body into a token request
	token := token{}
	json.NewDecoder(resp.Body).Decode(&token)

	// Check if request was successful
	if token.AccessToken == "" {
		return errors.New("CannotGetAccessToken")
	}

	c.config.AccessToken = token.AccessToken
	c.config.ExpiresAt = token.ExpiresIn + time.Now().Unix() - 10
	return nil

}

// Purchase structure
type Purchase struct {
	ConsumptionState int    `json:"consumptionState"`
	PurchaseState    int    `json:"purchaseState"`
	Kind             string `json:"kind"`
	DeveloperPayload string `json:"developerPayload"`
	PurchaseTime     int    `json:"purchaseTime"`
}

// Validate a purchase
func (c *Client) PurchaseValidate(packageName string, productId string, purchaseToken string) (p Purchase, err error) {
	endpoint := NewEndpoint("validatePurchase")
	endpoint.setOption("packageName", packageName)
	endpoint.setOption("productId", productId)
	endpoint.setOption("purchaseToken", purchaseToken)
	endpoint.setAccessToken(c.config.AccessToken)

	err = c.requestTo(endpoint, &p)
	return p, err
}

// Subscription status
type Subscription struct {
	Kind                    string `json:"kind"`
	InitiationTimestampMsec int    `json:"initiationTimestampMsec"`
	ValidUntilTimestampMsec int    `json:"validUntilTimestampMsec"`
	AutoRenewing            bool   `json:"autoRenewing"`
}

// Get status of a subscription
func (c *Client) SubscriptionGet(packageName string, subscriptionId string, purchaseToken string) (s Subscription, err error) {
	endpoint := NewEndpoint("getSubscriptionStatus")
	endpoint.setOption("packageName", packageName)
	endpoint.setOption("subscriptionId", subscriptionId)
	endpoint.setOption("purchaseToken", purchaseToken)
	endpoint.setAccessToken(c.config.AccessToken)

	err = c.requestTo(endpoint, &s)
	return s, err
}

// Cancel a subscription
func (c *Client) SubscriptionCancel(packageName string, subscriptionId string, purchaseToken string) error {
	endpoint := NewEndpoint("cancelSubscription")
	endpoint.setOption("packageName", packageName)
	endpoint.setOption("subscriptionId", subscriptionId)
	endpoint.setOption("purchaseToken", purchaseToken)
	endpoint.setAccessToken(c.config.AccessToken)

	err := c.requestTo(endpoint, nil)
	return err
}

// Create a new request and loads it's output into the `output` variable(second argument)
func (c *Client) requestTo(endpoint Endpoint, output interface{}) error {
	url := endpoint.generate()
	// Initialize a new request
	log.Println("Url:", url)
	req, _ := http.NewRequest("GET", url, nil)
	log.Println("Req:", req)
	client := &http.Client{}
	log.Println("Client:", client)
	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("CannotSendRequest")
	}
	log.Println("Resp:", resp)

	defer resp.Body.Close()

	// Check if request was successful
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 404 {
			return inappbillingerror.ErrTransactionNotFound
		}
		return errors.New("CannotGetData")
	}

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.New("CannotReadBody")
	}

	// Check access token expiration
	if string(body) == "Access token has been expired" {
		return errors.New("AccessTokenExpired")
	}

	// Check for cancel subscription
	if endpoint.Route == "cancelSubscription" && string(body) == "" {
		return nil
	}

	// Check if response was empty(this means that the server cannot find the resource!)
	if isEmpty, _ := regexp.Match("{\\s+}", body); isEmpty {
		return errors.New("CannotFindResource")
	}

	// Parse json response and load it into the `output` variable
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("Cannot parse JSON response")
	}

	return nil
}
