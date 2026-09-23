# AGENTS.md - zeebe-chaos

This repository has two parts: `go-chaos/`, the `zbchaos` fault-injection CLI used to run chaos
experiments against Zeebe, and `chaos-days/`, the write-ups of experiments run with it, published
as a Docusaurus blog at https://camunda.github.io/zeebe-chaos/.

---

## Repository layout

```
go-chaos/
  cmd/            # zbchaos CLI commands (cluster, terminate, disconnect, backup, stress, verify, ...)
  internal/       # chaos-experiment library, BPMN helpers, k8s manifests
  worker/         # load-generation workers used by experiments
  deploy/         # deployment manifests for experiment targets
  integration/    # integration tests
chaos-days/
  blog/<date>-<title>/index.md   # one experiment write-up per post
  templates/YYYY-MM-DD-template.md  # post skeleton
  newPost.sh                     # scaffolds a new post from the template
```

`go-chaos/` is built, tested, and released via `.github/workflows/go-ci.yml` and `release.yaml`
(see `go-chaos/README.md` for the `make build`/`make test`/`release.sh` commands). The blog is
built and published via `.github/workflows/buildChaosBlog.yml` and `publish-blog.yml`.

To start a new chaos-day post, run `chaos-days/newPost.sh "<Title>"` from `chaos-days/`. It copies
`templates/YYYY-MM-DD-template.md` into `blog/<current-date>-<title>/index.md`, substituting the
date and title.

A post's frontmatter carries `layout`, `title`, `date`, `categories`, `tags`, and `authors` (one
name or a list). The body follows `# Chaos Day Summary`, a **TL;DR;** paragraph, `<!--truncate-->`
(everything above is the blog list preview), then one `## Chaos Experiment` section per experiment
with `### Expected` / `### Actual` subsections, and a `## Found Bugs` section for anything
uncovered.

---

## Behavior rules

### Feed sizing findings back into camunda-docs

When a chaos-day experiment changes a sizing or capacity recommendation — memory, disk, CPU
headroom, replica counts, timeouts under resource pressure, or similar — check whether
[camunda-docs's self-managed sizing guide](https://github.com/camunda/camunda-docs/blob/main/docs/components/best-practices/architecture/sizing-self-managed.md)
needs a matching update, and open a `camunda-docs` PR or issue referencing the chaos-day post.

### Distill durable findings into the sizing knowledge base

A blog post here is a narrative, point-in-time record of one experiment. When a finding is a
reusable, generalized fact worth outliving that single post — a sizing rule of thumb, a measured
threshold, a durable behavioral note — also add it to
[`team-reliability-testing`'s `knowledge/sizing/`](https://github.com/camunda/team-reliability-testing/tree/main/knowledge/sizing),
which is the durable home for such facts, and link back to the chaos-day post as the source
experiment.

---

## Ownership

Owned by `@camunda/reliability-testing` (`.github/CODEOWNERS`).
