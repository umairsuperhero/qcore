package udm

// 3GPP TS 29.503 response types. We ship only the subset of each schema
// that QCore actually populates today; more fields get added as NFs
// downstream start consuming them. Staying close to the spec field names
// (camelCase JSON) so a stock 3GPP client speaks to us unmodified.

// AccessAndMobilitySubscriptionData — TS 29.503 §6.1.6.2.2.
//
// Returned by GET /nudm-sdm/v2/{supi}/am-data. AMF reads this during
// Registration to learn the UE's allowed NSSAI, UE-AMBR, GPSIs, and any
// RAT/access-type restrictions.
type AccessAndMobilitySubscriptionData struct {
	Gpsis            []string   `json:"gpsis,omitempty"`
	SubscribedUeAmbr *AmbrRm    `json:"subscribedUeAmbr,omitempty"`
	Nssai            *Nssai     `json:"nssai,omitempty"`
	RatRestrictions  []string   `json:"ratRestrictions,omitempty"`
}

// AmbrRm — TS 29.571 §5.4.2.6. Aggregate Maximum Bit Rate, removable
// (i.e. nullable) variant used inside subscription data.
type AmbrRm struct {
	Uplink   string `json:"uplink"`   // e.g. "1 Gbps"
	Downlink string `json:"downlink"` // e.g. "1 Gbps"
}

// Nssai — TS 29.571 §5.4.4.1. Network Slice Selection Assistance
// Information. We expose only defaultSingleNssais for v0.5.
type Nssai struct {
	DefaultSingleNssais []Snssai `json:"defaultSingleNssais,omitempty"`
	SingleNssais        []Snssai `json:"singleNssais,omitempty"`
}

// Snssai — TS 29.571 §5.4.4.2. Single NSSAI: SST (slice/service type,
// 0–255) and optional SD (slice differentiator, 6 hex digits).
type Snssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd,omitempty"`
}
