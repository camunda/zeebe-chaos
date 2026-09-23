# AGENTS.md - zeebe-chaos

This repository stores Zeebe/Camunda chaos-day experiment write-ups, published as a Docusaurus
blog at https://camunda.github.io/zeebe-chaos/.

---

## Repository layout

```
chaos-days/
  blog/<date>-<title>/index.md   # one experiment write-up per post
  templates/YYYY-MM-DD-template.md  # post skeleton
  newPost.sh                     # scaffolds a new post from the template
go-chaos/                        # Go chaos-toolkit experiments and tooling
```

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

This already happened once, organically: the
[slow disk on primary/secondary storage post](chaos-days/blog/2026-06-19-Using-slow-disk-with-Camunda/index.md)
led to [camunda-docs#9150](https://github.com/camunda/camunda-docs/pull/9150) and
[camunda-docs#9154](https://github.com/camunda/camunda-docs/pull/9154).

### Distill durable findings into the sizing knowledge base

A blog post here is a narrative, point-in-time record of one experiment. When a finding is a
reusable, generalized fact worth outliving that single post — a sizing rule of thumb, a measured
threshold, a durable behavioral note — also add it to
[`team-reliability-testing`'s `knowledge/sizing/`](https://github.com/camunda/team-reliability-testing/tree/main/knowledge/sizing),
which is the durable home for such facts, and link back to the chaos-day post as the source
experiment.

---

## Ownership

`.github/CODEOWNERS` currently lists a single individual (`@ChrisKujawa`) as owner of everything in
this repo, not a team.
