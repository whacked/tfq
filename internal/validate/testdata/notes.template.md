```cue
date:    string & =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
author:  string
slug:    string & =~"^[a-z0-9-]+$"
tags?:   [...string]
```

# <title>
