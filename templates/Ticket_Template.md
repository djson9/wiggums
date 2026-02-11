---
Date: <% tp.date.now("YYYY-MM-DD HH:mm") %>
Status: created
Agent:
MinIterations:
CurIteration: 0
SkipVerification: true
UpdatedAt:
---
## Original User Request


## Additional User Request
To be populated with further user request

```button
name Add Request
type append template
action AddUserRequest
```

```button
name Move to tickets
type append template
action MoveToTickets
remove true
```

```
<IMPORTANT AGENT INSTRUCTIONS>
When working on additional requests, please append ABOVE the first request. So we should have:

Request 3
- Findings/Results
- Execution Plan
- ...

Request 2
...

Request 1
...

We should be creating a new entry for each additional request.
</IMPORTANT AGENT INSTRUCTIONS>
```


---
Below to be filled by agent. Agent should not modify above this line.

# Request 1
## Findings / Results
TODO
## Execution Plan
TODO

## Additional Context
TODO

## Commands Run / Actions Taken
TODO

## Verification Commands / Steps
TODO

## Verification Coverage Percent and Potential Further Verification
TODO