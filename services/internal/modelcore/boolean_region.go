package modelcore

// BooleanRegionRef is a proof over exact source topology. Frozen digests may
// disambiguate equal boundary proofs only while both operands remain identical.
type BooleanRegionRef struct {
	BoundaryUnique bool     `json:"boundaryUnique"`
	Boundary       []string `json:"boundary"`
	InputDigest    string   `json:"inputDigest"`
	CellDigest     string   `json:"cellDigest"`
}
type BooleanResultSelection struct {
	Mode    string             `json:"mode"` // ALL or EXPLICIT; absent definition means ALL.
	Regions []BooleanRegionRef `json:"regions,omitempty"`
}
