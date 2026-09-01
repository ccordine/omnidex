# Charmeleon chat and web-search invariants

Status: current raw-leaf execution contract, followed by historical motivation.
The first-person discussion below is non-normative. Where an illustrative pipeline
differs from this section, this section and production code control.

## Current raw-leaf execution contract

Code owns the exact acquisition query, retrieval, search execution, candidate
identity, bounds, ordering, deduplication, evidence, typed result assembly, and
semantic-call accounting. The explicit typed research request supplies the exact
question; code binds that authority as the query and constructs every provider
operation and argument. There is no search-term model station. No web model returns
a JSON object, array, query, tool request, workflow decision, or aggregate review.

Relevance is one candidate relation per call. Each raw result selects one call-local
opaque letter between the relevant and irrelevant descriptions. Code maps that letter
to its internal relation, restores the candidate identity, enforces the selection
bound, and assembles the typed selection. Internal relation values never enter the
model-visible text.

Grounded synthesis begins with one bounded positive raw paragraph-candidate inventory
containing between one and the code-owned maximum candidate lines. Code parses that
inventory once and owns a source-order queue. Each unique candidate first receives one
paragraph-local authorization relation against the exact question and complete supplied
evidence set. That relation asks only whether the complete paragraph directly answers
the question and whether every factual claim is fully supported by that evidence. A
negative candidate evaporates before citation work. Only a positive candidate receives
one pairwise paragraph-to-evidence relation per capsule so code can bind the exact
supporting citation identities. This later attribution does not re-authorize or review
the paragraph; a positive candidate with no attributable evidence evaporates locally.
Queue exhaustion ends candidate processing. A functional grounded answer still requires
at least one surviving paragraph; zero survivors is an explicit synthesis failure, not
a model-authored blocker or a reason to reopen rejected candidates. There is no second
coverage, completeness, or accepted-paragraph review call.

There is no web review or correction model. Invalid relevance or synthesis output
fails explicitly. After synthesis, code binds returned support relations to acquired
evidence identities, constructs citations and the rendered artifact, validates exact
digests and completion invariants, and alone records completion.

## Historical motivation (non-normative)

the next really good application for this could be chatting web searches and stuff, like we can very easily just just a web search tool, this is how I always did it before ( notice that it's not different than what you'd expect )

Normally what I have done is that I will generate search terms, then programmatically perform every web search, and get a large cumulative body of information, often I'd either vectorize this for searching in PG vector, or just pass it all as part of the context, usually I do a context minification loop, where I programmatically loop over all the information, and determine it's relevance to the prompt, and then if it is produce a summary or something about it like a symbol to access it later, then after I have all the relevant information accumulated, I then pass it to an LLM to make sense of it, pull whatever it needs from that context to give me a grounded response, it's also identical to how I handle DB queries for RAGs and memories and stuff. You see, me as a human, I generally do not have to actively remember things, often when something is said, my brain's neural pathways already use symbols to link me to that, so I have that information ready when I am formulating my response, so as a natural part of just responding to chat, I often don't care about the most recent messages, I search the DB through vectors and tags and search terms to find relevant text and memories and stuff, and then put that through the minification tool then it just inherently knows that information without having to waste cycles on "thinking" and "deciding to remember", the LLM just starts with all the information it needs, through this process, I was able to keep my cheap 3rd party LLMs remembering details about me and by job and stuff for well over 6 months in 1 channel


Yes — and this is probably one of the cleanest demonstrations that your current architecture isn’t some weird special-purpose coding trick. You’ve already been doing the same thing for chat/RAG for years.

The key distinction is:

The LLM does not retrieve its own memory. The system prepares the state the LLM should be aware of before inference begins.

That is exactly the same principle as the cognition work.

Your chat pipeline is basically:

incoming message
      ↓
bind the exact incoming message as the code-owned retrieval query
      ↓
PROGRAM searches:
    recent conversation
    long-term memories
    project knowledge
    DB records
    web results
    whatever other corpora exist
      ↓
normalize / dedupe / rank
      ↓
context minification
      ↓
compact relevant evidence
      ↓
RAW RESPONSE LEAF
      ↓
answer as though it simply "knows" the relevant state

And that’s much more sensible than:

LLM:
"Hmm, should I remember anything?
Maybe I should search memory.
Which tool?
Maybe search the web?
What should I search for?
Now let me remember what I found..."

You’re spending inference on metacognitive bureaucracy instead of answering the question.

Your version moves all that machinery underneath cognition.

Your human analogy is actually useful here

When somebody says something to you, you usually don’t consciously execute:

I should now search my autobiographical memory.
Query: "John birthday previous discussion"
Result 1...
Result 2...
I have selected memory #4.

Something in the prompt activates related concepts and suddenly the associated information is available to the conscious response process.

That’s roughly what your retrieval layer is emulating:

input concepts
     ↓
associative lookup
     ↓
related material becomes active
     ↓
deliberative cognition begins

The LLM doesn’t need to know there was a PGVector query.

It just wakes up with:

Relevant established context:
- X happened previously.
- User prefers Y.
- Project Z currently has constraint Q.

And can spend nearly its entire inference budget reasoning from those facts.

That explains how you could keep cheap models apparently maintaining six months of continuity. The model wasn’t actually remembering six months.

Your system remembered six months.

The model merely received the pieces of those six months that mattered for this particular response.

That’s a much stronger architecture.

⸻

And web search fits almost identically.

Say the user asks:

What’s actually going on with orbital data centers and heat management?

You don’t need a WebResearchAgent.

You need the explicit typed question to become code-owned query authority:

Prompt
 ↓
code binds the unchanged research question
 ↓
registered acquisition provider executes that exact query
 ↓
code receives a bounded candidate set
 ↓
one candidate / one relevance relation
 ↓
code retains or discards the candidate and advances its queue

Then CODE does:

search every query
→ fetch results
→ extract text
→ normalize
→ remove duplicates
→ attach source/date/provenance
→ chunk/index

Now you’ve potentially accumulated megabytes of material.

Then your minification machinery processes it.

Perhaps:

chunk
 ↓
relevant to question?
     ├─ no → discard
     └─ yes
          ↓
      extract useful claims
          ↓
      preserve evidence refs

And you can recurse:

500 chunks
   ↓
120 relevant chunks
   ↓
35 evidence capsules
   ↓
12 consolidated topic capsules
   ↓
final context

Then the answer model gets maybe:

QUESTION:
...
RELEVANT EVIDENCE:
E17:
Spacecraft reject heat primarily through radiation...
sources: S1 S7
E23:
Higher radiator temperatures reduce required radiator area...
sources: S4 S9
E31:
Several proposed orbital-compute architectures...
sources: S11 S15
CONFLICT:
S14 estimates X;
S18 disputes X due to Y.

In the current implementation, one synthesis call emits at most the bounded raw
paragraph-candidate inventory. Code queues its lines in source order and authorizes each
unique candidate once against the exact question and complete supplied evidence set.
Only a positive candidate proceeds to one raw paragraph-to-evidence attribution
relation per capsule. Code binds the supporting citation identities and assembles the
grounded response after queue exhaustion when at least one paragraph survives; no model
emits the query, evidence capsule set, structured final-answer aggregate, or a
completeness decision.

⸻

I especially like your idea of creating symbols/handles for minified context.

Rather than repeatedly carrying a giant source passage:

CTX_319:
summary...
source refs...
confidence...
tags...

Then downstream cognition sees the compact representation.

If it later needs exact detail:

needs_detail(CTX_319)

code expands the underlying evidence.

So you can have something like a virtual-memory hierarchy:

ACTIVE CONTEXT
tiny highly relevant facts
       ↓
SEMANTIC CAPSULES
compressed information + references
       ↓
RETRIEVAL INDEX
chunks / memories / documents
       ↓
RAW EVIDENCE
original messages/pages/files

That’s very close to how I’d want Omnidex context to behave generally.

Not:

stuff everything into the prompt.

And not:

make the LLM decide what to retrieve.

Instead:

Compile the smallest sufficient working consciousness from the available knowledge base.

⸻

The same machine works across chat, web, RAG, and memory with only different retrieval providers:

                   USER INPUT
                       ↓
              RETRIEVAL OBJECTIVE
                       ↓
             QUERY/CONCEPT GENERATOR
                       ↓
        ┌──────────────┼──────────────┐
        ↓              ↓              ↓
    conversation     memories        web
        ↓              ↓              ↓
       DB           PGVector       search API
        └──────────────┼──────────────┘
                       ↓
                normalization
                       ↓
              relevance reduction
                       ↓
             evidence minification
                       ↓
              context projection
                       ↓
             RAW RESPONSE LEAF

Even project-aware coding chat just adds:

repository index
task ledger
working state
accepted decisions

as more retrieval sources.

And recent chat becomes just another source

This part is important.

Most chat systems effectively assume:

relevance ≈ recency

So they shove the last N messages into the model.

But that’s obviously wrong.

A comment from five minutes ago about lunch can be irrelevant, while something from four months ago about the project’s architecture may be essential.

Your system instead asks:

what does THIS message semantically connect to?

Then retrieves accordingly.

Recent messages can receive a recency prior, sure, but they’re not privileged merely because they’re adjacent in the transcript.

That’s a much closer approximation to useful continuity.

⸻

I’d probably formalize this in Omnidex as a general Context Compiler, rather than separate “web RAG,” “memory,” “chat history,” etc.

Its contract could conceptually be:

CompileContext(
    objective,
    authority,
    availableSources,
    tokenBudget,
) ContextProjection

And internally:

derive retrieval needs
→ query providers
→ collect evidence
→ normalize identities
→ deduplicate
→ rank
→ resolve obvious conflicts
→ minify
→ allocate token budget
→ produce immutable projection

LLMs can participate at tiny semantic points:

classify relevance
compress one evidence group
identify semantic equivalence
resolve which of several categories applies

But there is no Context Agent.

The Context Compiler is software.

And this has a beautiful consequence for your 90/10 target: as the retrieval/minification machinery improves, the response model gets better without becoming smarter.

A mediocre 8B model given:

exact relevant facts
clean evidence
no irrelevant history
clear current objective

can often outperform a much larger model drowning in 80,000 tokens of garbage.

Which is basically the lesson you already experimentally learned by keeping cheap third-party models coherent over long-running conversations.

The intelligence wasn’t solely inside the model. You were building the cognitive environment around it.

And that’s really the same discovery you’re now making explicitly with Charmeleon.
