package amazing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"hash"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var serviceDomains = map[string][]string{
	"CA": []string{"ecs.amazonaws.ca", "xml-ca.amznxslt.com"},
	"CN": []string{"webservices.amazon.cn", "xml-cn.amznxslt.com"},
	"DE": []string{"ecs.amazonaws.de", "xml-de.amznxslt.com"},
	"ES": []string{"webservices.amazon.es", "xml-es.amznxslt.com"},
	"FR": []string{"ecs.amazonaws.fr", "xml-fr.amznxslt.com"},
	"IT": []string{"webservices.amazon.it", "xml-it.amznxslt.com"},
	"JP": []string{"ecs.amazonaws.jp", "xml-jp.amznxslt.com"},
	"UK": []string{"ecs.amazonaws.co.uk", "xml-uk.amznxslt.com"},
	"US": []string{"ecs.amazonaws.com", "xml-us.amznxslt.com"},
}

var responseGroups = []string{"Request",
	"ItemIds",
	"Small",
	"Medium",
	"Large",
	"Offers",
	"OfferFull",
	"OfferSummary",
	"OfferListings",
	"PromotionSummary",
	"Variations",
	"VariationImages",
	"VariationSummary",
	"VariationMatrix",
	"VariationOffers",
	"ItemAttributes",
	"Tracks",
	"Accessories",
	"EditorialReview",
	"SalesRank",
	"BrowseNodes",
	"Images",
	"Similarities",
	"Reviews",
	"PromotionalTag",
	"AlternateVersions",
	"Collections",
	"ShippingCharges",
}

const (
	resourcePath    = "/onca/xml"
	resourceService = "AWSECommerceService"
	resourceVersion = "2011-08-01"
	envTag          = "AMAZING_ASSOCIATE_TAG"
	envAccess       = "AMAZING_ACCESS_KEY"
	envSecret       = "AMAZING_SECRET_KEY"
)

type Amazing struct {
	Config *AmazingClientConfig
	Client *http.Client
}

type AmazingClientConfig struct {
	ServiceDomain  []string
	AssociateTag   string
	AWSAccessKeyId string
	AWSSecretKey   string
}

func NewAmazing(domain, tag, access, secret string) (*Amazing, error) {
	return newAmazing(domain, tag, access, secret, nil)
}

func NewAmazingWithClient(domain, tag, access, secret string, client *http.Client) (*Amazing, error) {
	return newAmazing(domain, tag, access, secret, client)
}

func NewAmazingFromEnv(domain string) (*Amazing, error) {
	tag := os.Getenv(envTag)
	access := os.Getenv(envAccess)
	secret := os.Getenv(envSecret)

	if tag == "" || access == "" || secret == "" {
		return nil, fmt.Errorf("Can't read configuration from environment variables, are they set? "+
			"%s: %s\n"+
			"%s: %s\n"+
			"%s: %s\n", envTag, tag, envAccess, access, envSecret, secret)
	}

	return newAmazing(domain, tag, access, secret, nil)
}

func newAmazing(domain, tag, access, secret string, client *http.Client) (*Amazing, error) {
	if d, ok := serviceDomains[domain]; ok {
		config := &AmazingClientConfig{
			ServiceDomain:  d,
			AssociateTag:   tag,
			AWSAccessKeyId: access,
			AWSSecretKey:   secret,
		}
		if client == nil {
			client = NewTimeoutClient(time.Duration(3*time.Second), time.Duration(3*time.Second))
		}
		return &Amazing{config, client}, nil
	}
	return nil, fmt.Errorf("Service domain does not exist %v", serviceDomains)
}

func (a *Amazing) MergeParamsWithDefaults(extra url.Values) url.Values {
	params := url.Values{
		"AWSAccessKeyId": []string{a.Config.AWSAccessKeyId},
		"AssociateTag":   []string{a.Config.AssociateTag},
		"Service":        []string{resourceService},
		"Timestamp":      []string{time.Now().Format(time.RFC3339)},
		"Version":        []string{resourceVersion},
	}
	for k, v := range extra {
		params[k] = v
	}

	// attach signature
	signThis := fmt.Sprintf("GET\n%s\n%s\n%s", a.Config.ServiceDomain[0], resourcePath, strings.Replace(params.Encode(), "+", "%20", -1))
	h := hmac.New(func() hash.Hash {
		return sha256.New()
	}, []byte(a.Config.AWSSecretKey))
	h.Write([]byte(signThis))
	signed := base64.StdEncoding.EncodeToString(h.Sum(nil))
	params.Set("Signature", signed)

	return params

}

func (a *Amazing) ItemLookup(ctx context.Context, params url.Values) (*AmazonItemLookupResponse, error) {

	var result AmazonItemLookupResponse
	err := a.Request(ctx, params, &result)
	return &result, err

}

func (a *Amazing) ItemLookupAsin(ctx context.Context, asin string, extra url.Values) (*AmazonItemLookupResponse, error) {

	params := url.Values{
		"ResponseGroup": []string{"All"},
		"IdType":        []string{"ASIN"},
		"ItemId":        []string{asin},
		"Operation":     []string{"ItemLookup"},
	}

	if extra != nil {
		for k, v := range extra {
			params[k] = v
		}
	}

	return a.ItemLookup(ctx, params)

}

func (a *Amazing) ItemSearch(ctx context.Context, params url.Values) (*AmazonItemSearchResponse, error) {

	var result AmazonItemSearchResponse
	err := a.Request(ctx, params, &result)
	return &result, err

}

func (a *Amazing) SimilarityLookup(ctx context.Context, params url.Values) (*AmazonSimilarityLookupResponse, error) {

	var result AmazonSimilarityLookupResponse
	err := a.Request(ctx, params, &result)
	return &result, err

}

func (a *Amazing) Request(ctx context.Context, params url.Values, result interface{}) error {
	merged := a.MergeParamsWithDefaults(params)

	u := url.URL{
		Scheme:   "http",
		Host:     a.Config.ServiceDomain[0],
		Path:     resourcePath,
		RawQuery: merged.Encode(),
	}
	r, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return err
	}

	if ctx != nil {
		r = r.WithContext(ctx)
	}

	res, err := a.Client.Do(r)
	if err != nil {
		return fmt.Errorf("request, url: %v error: %v", u.String(), err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("read err body: %v", err)
		}

		var errorResponse AmazonItemLookupErrorResponse
		err = xml.Unmarshal(b, &errorResponse)
		if err != nil {
			return fmt.Errorf("unmarshal err: %v, url: %v, status: %v", err, res.Request.URL, res.StatusCode)
		}
		if errorResponse.Code == "RequestThrottled" {
			time.Sleep(time.Second)
			return a.Request(ctx, params, result)
		}
		return &errorResponse
	}

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read body: %v", err)
	}

	err = xml.Unmarshal(b, result)
	if err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}

	return nil
}
