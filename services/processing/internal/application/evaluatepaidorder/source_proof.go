package evaluatepaidorder

// SourceProof carries unverified source provenance separately from local policy
// pins and financial facts. It grants no sender authenticity, authority or digest
// verification. Legacy fixture readers may leave it nil. Values are owned by the
// returned snapshot and must not be mutated concurrently with consumers.
type SourceProof struct {
	FactsID, Digest                           string
	SessionCreatedAt                          *string
	OldestObservedAt, CompletedAt, ValidUntil string
}
