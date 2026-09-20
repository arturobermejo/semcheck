# semcheck

A linter for Go whose rules are questions in plain English.

Static analysis decides **where to look**: deterministic matchers on the syntax tree select exact nodes, such as a log call or a function named `GetSomething`. A decision model decides **whether the code is right**: it answers a closed question about each node with a probability, and semcheck reports the ones above a threshold.

Two rules, as they are written in `.semcheck.yml`:

```yaml
rules:
  - name: no-pii-in-logs
    match: { call: [log.Print*, slog.Info*, slog.Warn*, slog.Error*] }
    ask: "Does this log call write personal data (names, emails, phone numbers, postal addresses, government IDs)?"
    context: statement
    min_confidence: 0.8

  - name: name-matches-behavior
    match: { func-prefix: [Get, Is, Has, Find, List, Count] }
    ask: "Does this function modify state although its name suggests it only reads?"
    min_confidence: 0.8
```

**`no-pii-in-logs`** finds the four:

```go
slog.Info("user created", "email", u.Email)     // a rule on names finds it: the key is "email"
slog.Info("user created", "contact", u.Contact) // it takes knowing that Contact has Email and Phone
log.Printf("created %+v", u)                    // it takes knowing the fields of User
log.Printf("welcome, %s", fullName)             // the data is inside the message
```

The syntax tree gives it the calls, without missing one. The type checker gives it what the text does not say: that `u` is a `User` with an `Email`. The model reads the call **with those notes**, and answers one question.

**`name-matches-behavior`** finds the function whose name says "I only look":

```go
func IsExpired(u *User) bool {
	if time.Since(u.seen) > time.Hour {
		delete(users, u.ID) // whoever calls IsExpired does not expect to lose the user
		return true
	}
	return false
}
```

The matcher selects every function that starts with `Is`, `Get` or `Has`; the model reads each one whole and says which ones write. No list of "functions that write" could be complete: on real code it found a `GetOrgTokenIfExists` that deletes a file and a `getQueryTableInfo` that creates a database view.

On three open source projects, everything these two rules reported in the sample that was checked was right: see [Evaluation](#evaluation).

It runs by itself, as a `go vet` tool and as a [golangci-lint](https://golangci-lint.run) plugin, honors `//nolint:semcheck`, and keeps the answers of the model in a cache: a run only asks about the code that changed.

## Install and run

```bash
go install github.com/arturobermejo/semcheck/cmd/semcheck@latest
```

semcheck asks Jev, the decision model of TypeSafe AI, and reads the API key from `TYPESAFE_API_KEY`. It looks for `.semcheck.yml` in the current directory or the closest parent that has one; [the one of this repository](.semcheck.yml) is a starting point.

```bash
semcheck ./...                              # exit code 3 if there are findings
go vet -vettool=$(which semcheck) ./...
```

Without a key, and without sending anything anywhere:

```bash
semcheck -dry-run ./...                     # how many questions, and what they would cost
semcheck -dry-run -record=q.jsonl ./...     # every fragment of code that would be sent
SEMCHECK_JUDGE=fake:0.95 semcheck ./...     # a fake model that says yes to everything
```

| Flag | |
|---|---|
| `-config file` | the rules, instead of the closest `.semcheck.yml` |
| `-dry-run` | count the questions and estimate their cost, asking nothing |
| `-stats` | how many answers of each package came from the cache, and the tokens billed |
| `-record file` | add to the file every question, as a line of JSON, with its answer |

### As a golangci-lint plugin

semcheck is a [module plugin](https://golangci-lint.run/docs/plugins/module-plugins/): golangci-lint has to be built with it inside. [.custom-gcl.yml](.custom-gcl.yml) is the recipe and [.golangci.semcheck.yml](.golangci.semcheck.yml) a configuration that enables it.

```bash
golangci-lint custom                        # builds ./custom-gcl
./custom-gcl run
```

[.github/workflows/semcheck.yml](.github/workflows/semcheck.yml) is a GitHub Actions workflow that runs it, uploads the findings to GitHub code scanning as SARIF, and keeps the cache of answers from one run to the next. Here it only runs by hand; change its `on:` to run it on every pull request.

## Rules

| Field | | Default |
|---|---|---|
| `name` | what findings are reported as | required |
| `match` | the matcher that selects the nodes | required |
| `ask` | a question with a yes or no answer | required |
| `report_if` | the answer that makes a finding | `yes` |
| `min_confidence` | how sure the model must be, from 0 to 1 | `0.9` |
| `context` | what the model reads: the `statement` or the whole `function` around the node | `function` |
| `message` | what a finding says | made from `ask` |
| `tests` | look at `_test.go` files too | `false` |

| Matcher | Selects |
|---|---|
| `call: [patterns]` | calls to functions and methods, as `pkg.Func` with globs: `log.Print*`, `go.uber.org/zap.*`. Resolved with types, not by name |
| `func-prefix: [words]` | functions whose name starts with one of the words: `Get`, `Is`, `Has` |
| `exported-func-doc` | exported functions that have a doc comment |
| `test-func` | `TestXxx(*testing.T)` functions |

`fail_on_judge_error: true`, at the top of the file, makes the analysis fail when the model cannot answer. By default it warns and goes on: a model that is down does not turn every build red.

Writing a good question: ask about a **contradiction** the model can see in the fragment, not about something that is missing; be concrete; leave out adverbs such as "clearly". Then look at the numbers with `-record` before choosing `min_confidence`.

## Cost, speed and the cache

An answer is kept under a hash of the question, the code and its type notes, in the cache directory of the user (`SEMCHECK_CACHE` sets another, or `off`). Renaming a rule or moving its threshold costs nothing; changing a function asks about that function again.

Measured on three projects, 6,412 questions took 203 seconds and 3.75 million tokens: **16 cents** for a first run on 300,000 lines of Go. On its own code, after a change that touched half of the files, semcheck asked 30 questions of 201.

## Evaluation

Five rules were run on [caddy](https://github.com/caddyserver/caddy), [cloudflared](https://github.com/cloudflare/cloudflared) and [pocketbase](https://github.com/pocketbase/pocketbase), at fixed commits. A sample of 96 questions, stratified by what the model had answered, was then labeled without seeing those answers.

| Rule | Questions | Findings | Labeled | Right | Precision |
|---|---|---|---|---|---|
| `no-pii-in-logs` | 814 | 2 | 2 | 2 | 100% |
| `name-matches-behavior` | 491 | 25 | 10 | 10 | 100% |
| `log-level-fits` | 777 | 70 | 10 | 6 | 60% |
| `doc-matches-code` | 2,254 | 7 | 6 | 2 | 33% |
| `test-name-matches` | 2,076 | 11 | 10 | 1 | 10% |

What it found: two log calls that write the email address of the user who is signing in; a `GetOrgTokenIfExists` that deletes the token file; a `getQueryTableInfo` that creates and drops a database view; failures logged at debug level.

What came of it:

- **The first two rules ship as they are.**
- **`log-level-fits` ships with a higher threshold**, 0.7, where 6 of its 8 sampled findings were right.
- **The last two do not ship.** Right and wrong findings had the same probabilities, so no threshold tells them apart: the questions have to be written again. They are kept in [eval/rules.yml](eval/rules.yml).
- Nothing the model scored under 0.5 was labeled as a problem, in any rule. With four labels for strata of hundreds of questions, that says little about recall.

The labels were given by a large language model (Claude), not by a person; [eval/labels.jsonl](eval/labels.jsonl) has each one with its reason, for anyone to check. To repeat the evaluation, which needs an API key:

```bash
eval/run.sh              # clones the projects and asks: tmp/eval/results
go run ./eval            # tables of what the model answered
go run ./eval sample     # the 96 questions
go run ./eval label      # label them by hand; a label by hand replaces the one that is there
go run ./eval metrics
```

## What leaves your machine

Using Jev sends the code of every question, and the notes on its types, to the API of TypeSafe AI. semcheck only does it with an API key in the environment, and sends nothing else: not the file, not its path. `semcheck -dry-run -record=file` writes exactly what would be sent.

## Limits

- A rule sees what a matcher can select. "This file mixes HTTP and database code" cannot be written: tools that send whole files fit that better.
- The model reads one fragment. What a function does inside the functions it calls is not there.
- The model gives a probability, not a reason: a finding says which rule and how sure, not why.
- Answers vary by a few hundredths from one request to the next. The cache makes a run repeat itself; two machines without a shared cache may disagree on a finding that is right at the threshold.

## As a library

```go
cfg, err := semcheck.LoadConfig(".semcheck.yml")
judge, err := semcheck.DefaultJudge() // Jev, with the cache
analyzer, err := semcheck.NewAnalyzer(cfg, judge)
```

`Judge` is one method that answers a batch of questions with probabilities: any model can be put behind it.

## License

[MIT](LICENSE)
