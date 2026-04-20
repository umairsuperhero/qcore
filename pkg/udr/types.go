package udr

// TS 29.505 subscription-data response types. Same shape as the UDM
// types in pkg/udm — 3GPP defines them in a shared OpenAPI schema
// (TS 29.571 / TS 29.519) and each NF's OpenAPI references it. In Go
// we keep local copies per package for now; if the duplication starts
// hurting we'll factor into a pkg/sbi/models or generate from the
// 3GPP YAML.

// AccessAndMobilitySubscriptionData — TS 29.505 §6.1.6.2.2. UDR's
// version of AM data; UDM reads this and re-serves it verbatim under
// Nudm_SDM.
type AccessAndMobilitySubscriptionData struct {
	Gpsis            []string `json:"gpsis,omitempty"`
	SubscribedUeAmbr *AmbrRm  `json:"subscribedUeAmbr,omitempty"`
	Nssai            *Nssai   `json:"nssai,omitempty"`
	RatRestrictions  []string `json:"ratRestrictions,omitempty"`
}

// AmbrRm — TS 29.571 §5.4.2.6.
type AmbrRm struct {
	Uplink   string `json:"uplink"`
	Downlink string `json:"downlink"`
}

// Nssai — TS 29.571 §5.4.4.1.
type Nssai struct {
	DefaultSingleNssais []Snssai `json:"defaultSingleNssais,omitempty"`
	SingleNssais        []Snssai `json:"singleNssais,omitempty"`
}

// Snssai — TS 29.571 §5.4.4.2.
type Snssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd,omitempty"`
}
