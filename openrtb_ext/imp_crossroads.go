package openrtb_ext

// ExtImpCrossroads defines the contract for bidrequest.imp[i].ext.prebid.bidder.crossroads
type ExtImpCrossroads struct {
	PublisherId string   `json:"publisherId"`
	Floor       *float64 `json:"floor"`
}
