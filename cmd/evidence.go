package cmd

// liveRecord is the live validation record of 2026-10-14 (VER-005): this tool at
// 819a871 against GitLab EE 18.8 in Docker, with every call's argv, exit code and
// stdout checked against the contract, and a read-back outside the tool for every
// write (docs/evidence/2026-10-14-gitlab-ee-18.8.json is its machine-readable twin).
const liveRecord = "docs/evidence/2026-10-14-gitlab-ee-18.8.md"

// liveEvidence cites the section of liveRecord that holds a command group's live
// calls. A command that passed there is verified (CLI-027, VER-005); a change to its
// request, parsing or output voids the record, and the maturity goes back down.
func liveEvidence(section string) string { return "live:" + liveRecord + "#" + section }
