# Wide blocks

A narrow formula stays in the column:

\[
x^2 + y^2 = z^2
\]

A wide formula gets more width, centred on the window:

\[
\text{Human-advantage potential} = f(\text{long horizon}, \text{broad scope}, \text{poor verifiability}, \text{weak feedback}, \text{rare events}).
\]

A wide code block:

```
func veryLongFunctionName(firstArgument string, secondArgument int, thirdArgument map[string][]byte) (result string, err error) {
    return fmt.Sprintf("%s-%d-%d", firstArgument, secondArgument, len(thirdArgument)), nil
}
```

A short code block stays as it is:

```
x := 1
```

A squeezed table: its cells wrap in the text column, so it shows width buttons on hover:

| Hypothesis | Why it may hold | Why it may fail | How to check it |
|---|---|---|---|
| Long horizon tasks favour humans | Feedback comes long after the decision, so training sees little of it | Agents may learn proxies that track the long-term outcome well enough | Compare experts and agents on tasks with delayed, measured outcomes |
| Broad scope favours humans | The best action invests in future work outside the current task | Larger context windows may cover the wider scope | Give both sides a task series and score the whole series |

A small table does not need buttons:

| a | b |
|---|---|
| 1 | 2 |

A wide table:

| Column one | Column two | Column three | Column four | Column five | Column six | Column seven |
|---|---|---|---|---|---|---|
| some long cell text here | some long cell text here | some long cell text here | some long cell text here | text | text | text |

- A list item with a wide formula:

  \[
  \text{Human-advantage potential} = f(\text{long horizon}, \text{broad scope}, \text{poor verifiability}, \text{weak feedback}).
  \]

The end.
