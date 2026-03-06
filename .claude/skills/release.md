# Release Skill

## Description
Prepare and publish a new release of agent-corral to PyPI and create a GitHub release.

## Instructions

Follow these steps to release a new version:

### 1. Determine the version bump
- Review commits since the last version bump: `git log $(git log --oneline --all --grep="Bump version" -1 --format=%h)..HEAD --oneline --no-merges`
- Decide on semver bump (patch/minor/major) based on changes
- If the user specifies a version, use that

### 2. Bump the version
- Update `version` in `pyproject.toml`
- Update `RELEASE.md` with a new section at the top documenting the changes (follow existing format)

### 3. Build the package
```bash
cd /path/to/repo
python -m pip install build twine
python -m build
```
This creates `dist/agent_corral-X.Y.Z.tar.gz` and `dist/agent_corral-X.Y.Z-py3-none-any.whl`.

### 4. Verify the build
```bash
twine check dist/agent_corral-X.Y.Z*
```

### 5. Commit the version bump
Create a commit with message: `Bump version to X.Y.Z`

### 6. Ask the user before publishing
Before proceeding, confirm with the user that they want to:
- Publish to PyPI
- Create a GitHub release
- Push the commit and tag

### 7. Publish to PyPI
```bash
twine upload dist/agent_corral-X.Y.Z*
```
This requires PyPI credentials (token in `~/.pypirc` or `TWINE_USERNAME`/`TWINE_PASSWORD` env vars).

### 8. Tag and push
```bash
git tag vX.Y.Z
git push origin main
git push origin vX.Y.Z
```

### 9. Create GitHub release
```bash
# Extract the release notes for this version from RELEASE.md
gh release create vX.Y.Z --title "vX.Y.Z" --notes-file <(sed -n '/^## vX.Y.Z/,/^## v/{ /^## v[^X]/d; p }' RELEASE.md) dist/agent_corral-X.Y.Z*
```

### 10. Clean up
```bash
rm -rf dist/ build/ src/*.egg-info
```
