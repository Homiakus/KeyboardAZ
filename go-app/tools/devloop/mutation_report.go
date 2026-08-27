package main

import "encoding/json"

// UnmarshalJSON deliberately treats the Gremlins report as an external schema.
// Internal engineering manifests use DisallowUnknownFields, but mutation tools
// may add metadata such as go_module, elapsed_time or per-file details without
// changing the counters this controller evaluates.
func (r *mutationReport) UnmarshalJSON(data []byte) error {
	type reportAlias mutationReport
	var decoded reportAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = mutationReport(decoded)
	return nil
}
