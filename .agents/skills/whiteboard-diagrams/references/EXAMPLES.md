# Topology examples

Use these as structural patterns, not copy templates.

## Linear workflow

Request: explain a document-processing pipeline.

```json
{
  "title": "HOW A DOCUMENT BECOMES SEARCHABLE",
  "goal": "Show how an uploaded document becomes searchable chunks.",
  "lanes": [
    {
      "label": "PROCESSING",
      "flows": [
        "uploaded PDF -> extract text -> split into chunks -> create embeddings -> vector index"
      ]
    }
  ]
}
```

## Join

Request: explain retrieval-augmented generation.

```json
{
  "title": "HOW RAG ANSWERS A QUESTION",
  "goal": "Show how retrieved evidence joins the user's question before generation.",
  "lanes": [
    {
      "label": "RETRIEVE + ANSWER",
      "flows": [
        "question -> embedding -> vector search -> relevant passages",
        "question + relevant passages -> LLM -> cited answer"
      ]
    }
  ],
  "annotations": ["relevant passages: evidence, not model memory"]
}
```

## Parallel modes with a feedback loop

Use separate lanes when training and generation share components but tell different causal stories. Put the training feedback loop only in the training lane. See `examples/text-to-image-model.json`.

## Before and after

Request: explain a cache hit.

Use two short lanes or two aligned halves:

- `WITHOUT CACHE`: `request -> database -> response`
- `WITH CACHE`: `request -> cache hit -> response`

Do not add an arrow from the cache-hit response to the database because that reverses the point of the comparison.
