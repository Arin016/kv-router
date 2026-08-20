# SCALE: the whiteboard diagram standard

SCALE is a five-pass method for turning a prompt into a diagram that is accurate before it is attractive:

1. **Scope** the explanation.
2. **Connect** the causal model.
3. **Arrange** the page.
4. **Language** the visual system.
5. **Evaluate** the rendered image.

Read and apply every pass. Do not jump directly from a topic to image generation.

## 1. Scope

### Establish the teaching goal

Write one sentence that completes this phrase:

> After seeing this diagram, the viewer should understand ...

That sentence is the diagram's goal. If the request does not provide enough information to complete it honestly, ask the user what the diagram should explain.

### Identify the audience

Use the audience to decide vocabulary, not the underlying truth.

- **General:** prefer plain-language labels and one short technical parenthesis where necessary.
- **Technical:** keep canonical component names, tensor or data names, and exact boundaries.
- **Executive:** emphasize actors, decisions, inputs, outputs, and risk; omit implementation internals.
- **Classroom:** show intermediate states and a concise takeaway.

Default to a general, curious audience when no audience is specified.

### Build the content inventory

Extract these items from the user's prompt:

- title or topic;
- starting inputs;
- transformations or decisions;
- intermediate states;
- final outputs;
- branches, joins, loops, or repeated stages;
- important distinctions such as frozen/trainable, client/server, before/after, or human/model;
- exact labels and phrases that must appear;
- exclusions or claims that must not appear.

The subject and at least one meaningful relationship are required. Style details are not required.

### Set a complexity budget

Aim for one teachable idea per image.

- 5 to 12 named components is comfortable.
- 13 to 16 components requires lanes, grouping, or a larger canvas.
- More than 16 components usually deserves multiple diagrams unless the relationships are exceptionally simple.
- Keep most labels to five words or fewer.
- Use at most one footer takeaway.

Never remove a causally necessary step just to hit a number. Split the explanation instead.

## 2. Connect

### Model the diagram before drawing it

Write the workflow as text first. Every edge must answer one of these questions:

- What moves?
- What changes?
- What controls the next step?
- What combines here?
- What repeats?
- What is produced?

Use arrows only when direction matters. Use proximity or a loose hand-drawn enclosure for membership. Do not use arrows as decoration.

### Represent common relationships

- **Sequence:** `A -> B -> C`
- **Join:** `A + B -> C`
- **Branch:** `A -> B` and `A -> C`
- **Feedback:** `A -> B -> C`, then one curved return arrow from `C` to the exact repeated stage
- **Parallel phases:** separate labeled lanes that share the same left-to-right reading direction
- **State change:** name both states, such as `clean image + noise -> noisy image`
- **Boundary:** label the boundary and show only the edges that cross it

### Preserve truth under simplification

Simplification may remove detail, but it must not reverse or merge distinct operations.

- Distinguish training from inference when they use different inputs or loops.
- Distinguish a representation from the encoder that creates it.
- Distinguish a target from the model's prediction.
- Distinguish a frozen dependency from the component being trained.
- Show where conditioning enters, not merely that it exists.
- Point a feedback loop to the stage that actually repeats.

If the source prompt is ambiguous on any of these points and the ambiguity changes the causal story, ask the user.

### Create a label ledger

Before generation, list every string that must be rendered exactly. Shorten surrounding copy, not canonical names. Treat spelling, capitalization, hyphenation, units, and numerical values as invariants.

## 3. Arrange

### Choose topology from the causal shape

Do not default to boxes in a row when the system is not linear.

- Use a single horizontal flow for a true sequence.
- Use stacked horizontal lanes for two related modes such as training and generation.
- Use a central join when multiple inputs condition one operation.
- Use a loop only for a real repeated process.
- Use before/after halves for transformations with one clear transition.
- Use a compact hub only when every spoke genuinely shares one center.

### Default canvas

- 16:9 landscape.
- Warm white, untextured whiteboard surface.
- At least 7% clear margin on every edge.
- Title in the upper band, content below it.
- Left-to-right primary reading direction.
- The first and last nodes must sit fully inside the safe margin.

Use portrait only when the user requests it or the process is intrinsically vertical.

### Establish hierarchy

Use three typographic levels:

1. title;
2. lane or phase labels;
3. node labels and annotations.

The title should describe the whole picture. Lane labels name distinct modes, not vague categories. Annotations explain a special condition and must sit next to the affected component.

### Route edges cleanly

- Prefer short horizontal arrows.
- Keep arrows away from text and doodles.
- Avoid crossings. Reorder nodes before accepting a crossed edge.
- Merge related inputs at one visually obvious plus sign or junction.
- Use a single curved arrow for a loop, with its label on the open side of the curve.
- Give arrowheads enough room that they do not touch the destination label.

### Clear every edge

No title, arrowhead, word, frame, doodle, or footer may touch or cross the canvas boundary. Preserve extra room around the title and footer because hand-drawn strokes extend beyond their apparent baseline.

## 4. Language

### Medium

The default look is a real marker sketch on a physical whiteboard:

- dry-erase strokes with slight variation in pressure;
- human handwriting that remains highly legible;
- imperfect but confident arrows;
- simple object doodles only where they replace explanation;
- flat marks with no artificial depth.

The work should feel drawn by a strong teacher, not exported from a corporate diagram tool.

### Color grammar

Use color as syntax:

- **Black:** title, structure, primary labels, outlines.
- **Blue:** flow, data, arrows, intermediate representations.
- **Red:** one critical distinction, trainable component, warning, or decision.
- **Green or orange:** allowed only for a small literal output doodle when it improves recognition.

Never spread a saturated accent across unrelated elements. Never use gradients, glow, translucency, or shadow.

### Shapes

- Bare words are valid nodes when the concept is self-explanatory.
- Use a simple hand-drawn rectangle only when a boundary or component needs containment.
- Use a plus sign for a join.
- Use a tiny literal doodle for an image, person, document, database, or physical outcome.
- Do not place icons inside colored tiles.
- Do not wrap metadata in pills or chips.
- Do not fake a software window.

### Writing

- Say less.
- Prefer verbs for transformations: `compress`, `add noise`, `predict`, `compare`, `repeat`.
- Prefer concrete nouns for states: `text embedding`, `noisy image`, `new image`.
- Keep explanatory notes close to what they modify.
- Use one plain-language takeaway at the bottom only when it adds meaning.
- Avoid decorative quotation marks, marketing copy, jokes that compete with the lesson, and filler labels.

### Prompt construction

An image-generation prompt must explicitly include:

- asset type and aspect ratio;
- teaching goal and audience;
- exact title and required text;
- every lane, node, edge, join, and loop;
- color semantics;
- safe margins and no-clipping constraint;
- style and medium;
- prohibited treatments;
- the instruction that causal accuracy and exact spelling outrank decoration.

Use `scripts/diagram_prompt.py` to compile this structure from a JSON spec.

## 5. Evaluate

Inspect the generated image itself. A valid prompt is not proof of a valid diagram.

### Accuracy pass

- Every required component appears.
- Every arrow points in the correct direction.
- Joins combine the correct inputs.
- Loops return to the correct stage.
- Modes such as training and generation are not mixed.
- Special states such as frozen or trainable are attached to the correct component.

### Text pass

- All required text is present.
- Spelling and capitalization match the label ledger.
- No label is duplicated accidentally.
- No invented technical term appears.
- Text remains legible at normal viewing size.

### Composition pass

- The reading order is immediate.
- Related content is grouped.
- Parallel lanes align.
- No arrow crosses a label.
- Nothing is clipped.
- Margins are visibly generous.
- The result is not crowded or dominated by empty space.

### Style pass

- The result looks genuinely hand drawn.
- Color follows meaning.
- There are no gradients, glows, shadows, UI cards, icon tiles, background grids, or fake app chrome.
- Doodles clarify rather than decorate.
- The image has one coherent visual voice.

### Accessibility pass

- The diagram remains understandable if red and blue appear similar.
- Color is never the only indicator of a distinction; pair it with a label such as `frozen` or `this learns`.
- Labels have strong contrast against the whiteboard.
- The title and takeaway are not needed to decode the arrows.

### Correction policy

If causality, required text, or clipping is wrong, correct and regenerate. Do not excuse it as a model limitation. Make the correction targeted:

1. name the exact defect;
2. restate the invariant;
3. ask the generator to change only that defect while preserving the rest;
4. inspect the new output again.

## Definition of done

A diagram is finished only when a first-time viewer can trace the complete story, every label and relationship matches the source, the whole composition is visible, and the whiteboard style supports rather than competes with the explanation.
