id?: string
status?: "draft" | "proposed" | "accepted" | "deprecated" | "rejected" | "superseded"
date?: string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}( [0-9]{2}:[0-9]{2}:[0-9]{2}.*)?$"
intent?: "normative" | "descriptive"
author?: string
owner?: string
tags?: [...string]
references?: [...string]
supersedes?: string
