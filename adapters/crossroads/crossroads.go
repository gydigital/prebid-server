package crossroads

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/prebid/prebid-server/v3/adapters"
	"github.com/prebid/prebid-server/v3/config"
	"github.com/prebid/prebid-server/v3/errortypes"
	"github.com/prebid/prebid-server/v3/openrtb_ext"
	"github.com/prebid/prebid-server/v3/util/jsonutil"
)

type CrossroadsAdapter struct {
	endpoint string
}

func processImp(imp *openrtb2.Imp) error {
	var ext adapters.ExtImpBidder
	var crext openrtb_ext.ExtImpCrossroads
	if err := jsonutil.Unmarshal(imp.Ext, &ext); err != nil {
		return err
	}
	if err := jsonutil.Unmarshal(ext.Bidder, &crext); err != nil {
		return err
	}
	if imp.Banner == nil && imp.Video == nil {
		return fmt.Errorf("neither Banner nor Video object specified")
	}
	imp.TagID = crext.PublisherId
	if crext.Floor == nil {
		return nil
	} else {
		imp.BidFloor = *crext.Floor
	}
	// no error
	return nil
}

func (a *CrossroadsAdapter) MakeRequests(request *openrtb2.BidRequest, extra *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	errs := make([]error, 0, len(request.Imp)+1)
	reqs := make([]*adapters.RequestData, 0, 1)
	crRequest := *request
	var validImps []openrtb2.Imp
	for _, imp := range crRequest.Imp {
		if err := processImp(&imp); err == nil {
			validImps = append(validImps, imp)
		} else {
			errs = append(errs, err)
		}
	}
	if len(validImps) == 0 {
		err := fmt.Errorf("No valid impressions for crossroads")
		errs = append(errs, err)
		return nil, errs
	}
	crRequest.Imp = validImps
	reqJSON, err := json.Marshal(crRequest)
	if err != nil {
		errs = append(errs, err)
		return nil, errs
	}
	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")

	if len(validImps) > 0 {
		endpoint := strings.Replace(a.endpoint, "${publisherId}", validImps[0].TagID, 1)

		reqs = append(reqs, &adapters.RequestData{
			Method:  "POST",
			Uri:     endpoint,
			Body:    reqJSON,
			Headers: headers,
			ImpIDs:  openrtb_ext.GetImpIDs(crRequest.Imp)})
	}

	return reqs, errs
}

func getBidCount(bidResponse openrtb2.BidResponse) int {
	c := 0
	for _, sb := range bidResponse.SeatBid {
		c = c + len(sb.Bid)
	}
	return c
}

func (a *CrossroadsAdapter) MakeBids(internalRequest *openrtb2.BidRequest, externalRequest *adapters.RequestData, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if response.StatusCode == http.StatusBadRequest {
		return nil, []error{&errortypes.BadInput{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode),
		}}
	}

	if response.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode)}
	}

	if len(response.Body) == 0 {
		return nil, []error{fmt.Errorf("Empty response body")}
	}

	glog.Infof("Crossroads response body: %s", string(response.Body))

	var bidResp openrtb2.BidResponse
	if err := jsonutil.Unmarshal(response.Body, &bidResp); err != nil {
		return nil, []error{fmt.Errorf("Error unmarshaling server response: %v. Response body: %s", err, string(response.Body[:min(100, len(response.Body))]))}
	}

	var errs []error
	count := getBidCount(bidResp)
	bidResponse := adapters.NewBidderResponseWithBidsCapacity(count)
	bidResponse.Currency = bidResp.Cur

	for _, sb := range bidResp.SeatBid {
		for i := 0; i < len(sb.Bid); i++ {
			bid := sb.Bid[i]
			bidType := getBidTypeFromBid(&bid)
			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &bid,
				BidType: bidType,
			})
		}
	}
	return bidResponse, errs
}

func getBidTypeFromBid(bid *openrtb2.Bid) openrtb_ext.BidType {
	if bid.MType == openrtb2.MarkupVideo {
		return openrtb_ext.BidTypeVideo
	}

	if bid.AdM != "" && strings.Contains(strings.ToUpper(bid.AdM), "<VAST") {
		return openrtb_ext.BidTypeVideo
	}

	return openrtb_ext.BidTypeBanner
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Builder(bidderName openrtb_ext.BidderName, config config.Adapter, server config.Server) (adapters.Bidder, error) {
	bidder := &CrossroadsAdapter{
		endpoint: config.Endpoint,
	}
	return bidder, nil
}
