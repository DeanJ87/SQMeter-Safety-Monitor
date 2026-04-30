# GitHub Repository Configuration

## ✅ Completed

- **Auto-delete branches on PR merge**: Enabled via `gh repo edit --delete-branch-on-merge`

## Branch Protection for `main`

To enable branch protection for the `main` branch, run this command:

```bash
gh api \
  --method PUT \
  /repos/DeanJ87/SQMeter-Safety-Monitor/branches/main/protection \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "checks": [
      {"context": "Lint and Test"},
      {"context": "Build"}, 
      {"context": "ASCOM Conformance"}
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": null,
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
```

Or configure manually in GitHub:
1. Go to **Settings** → **Branches**
2. Add a branch protection rule for `main`
3. Enable:
   - ☑️ Require status checks to pass before merging
     - ☑️ Require branches to be up to date before merging
     - Select: `Lint and Test`, `Build`, `ASCOM Conformance`
   - ☑️ Do not allow bypassing the above settings (optional)

## Alternative: Probot Settings App

Install the [Probot Settings app](https://probot.github.io/apps/settings/) on your repository to automatically apply the configuration from `.github/settings.yml`.
