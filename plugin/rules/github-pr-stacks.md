---
trigger: model_decision
description: "Native GitHub Pull Request Stack creation, update, and async merge workflow when stacking PRs on GitHub."
---

# Native GitHub Pull Request Stacking

When stacking pull requests on GitHub, `gh pr create --base <parent-branch>`
alone only sets the Git base branch reference and does **not** create a native
GitHub Pull Request Stack (`PullRequestStack`).

## 1. When to Register vs. Not Register a Stack

- **When to Register a Native GitHub PR Stack:** Whenever the user asks to
  **stack** a PR (e.g., *"stack it on top of PR #72"*, *"stack the PR"*,
  *"create a stacked PR"*) or when decomposing work into a logical chain of
  dependent **Stacked PRs** (`commit` Step 6), you **MUST** create or update the
  native GitHub Pull Request Stack via the GitHub API so GitHub's UI links the
  stack automatically.
- **When NOT to Automatically Create a Stack:** Do **not** blindly create a
  native GitHub PR Stack every time `--base <non-default-branch>` is used if the
  branch was merely based on in-flight work to avoid waiting and is not intended
  as a stacked PR chain.

## 2. Creating or Updating a Native GitHub PR Stack

1. **Inspect Existing Stacks on the Parent PR:**
   ```bash
   PARENT_PR=$(gh pr view "<parent-branch>" --json number --jq .number)
   gh api graphql -F owner="<owner>" -F name="<repo>" -F pr="$PARENT_PR" -f query='
     query($owner: String!, $name: String!, $pr: Int!) {
       repository(owner: $owner, name: $name) {
         pullRequest(number: $pr) {
           number
           ViewerPullRequestStacks {
             number
             id
             pullRequests { number }
           }
         }
       }
     }'
   ```
1. **Create a New Stack (if `ViewerPullRequestStacks` is empty):** Pass the PR
   numbers ordered from bottom/base-first to top/head-last:
   ```bash
   echo "{\"pull_requests\": [$PARENT_PR, $NEW_PR]}" | \
     gh api -X POST "repos/<owner>/<repo>/stacks" --input -
   ```
1. **Append to an Existing Stack (if a stack `<stack_number>` already exists):**
   ```bash
   echo "{\"pull_requests\": [<existing_pr_numbers_in_order>, $NEW_PR]}" | \
     gh api -X PATCH "repos/<owner>/<repo>/stacks/<stack_number>" --input -
   ```

## 3. Merging Stacked PRs

Standard GraphQL and synchronous REST merge endpoints reject stacked PRs with
HTTP 403. Merge stacked PRs using GitHub's asynchronous merge endpoint:

```bash
gh api -X PUT "repos/<owner>/<repo>/pulls/<pr_number>/merge-async" \
  -f merge_method=squash -f merge_action=direct_merge
```
