# Diagram intake contract

Create one JSON file before compiling a diagram prompt.

## Required fields

```json
{
  "title": "Exact diagram title",
  "goal": "What the viewer should understand",
  "lanes": [
    {
      "label": "PHASE OR MODE",
      "flows": [
        "input -> operation -> output",
        "second input + output -> next operation"
      ]
    }
  ]
}
```

- `title`: exact text for the heading.
- `goal`: one sentence describing the teaching goal.
- `lanes`: one or more modes, phases, or coherent regions.
- `lanes[].label`: exact lane heading.
- `lanes[].flows`: directional statements using `->` or `→`. Use `+` for joins.

## Optional fields

```json
{
  "audience": "general nontechnical audience",
  "format": "16:9 landscape",
  "annotations": [
    "CLIP: frozen",
    "100M U-NET: this learns"
  ],
  "loops": [
    "compare + improve -> 100M U-NET, labeled repeat"
  ],
  "exact_text": [
    "frozen",
    "this learns"
  ],
  "footer": "One concise takeaway.",
  "output_doodles": [
    "a tiny finished fox-and-tree image labeled new image"
  ],
  "avoid": [
    "equations",
    "brand logos"
  ]
}
```

Omitted optional fields receive SCALE defaults. Do not place a required technical fact only in `goal`; put it in a flow, annotation, loop, or exact-text entry so it becomes visible.

## Intake decision

Ask the user for clarification before creating this spec when either is unknown:

1. the goal or subject; or
2. the main components and how they connect.

Do not invent flow statements from a topic alone.
