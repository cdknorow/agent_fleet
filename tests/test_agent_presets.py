"""Tests for agent presets and team presets (predefined agent roles).

This feature adds:
- A set of built-in agent presets (Lead Developer, QA Engineer, Orchestrator)
- User-created custom agent presets (CRUD)
- Team presets that bundle multiple agent presets together
- API endpoints for managing presets from the dashboard
"""

import pytest
import pytest_asyncio

from coral.store import CoralStore


@pytest_asyncio.fixture
async def store(tmp_path):
    """Create a CoralStore backed by a temp DB."""
    s = CoralStore(db_path=tmp_path / "test.db")
    yield s
    await s.close()


# ── Built-in Agent Presets ─────────────────────────────────────────────────


EXPECTED_BUILTINS = [
    {
        "name": "Lead Developer",
        "prompt": None,  # Will check contains instead
        "is_builtin": True,
    },
    {
        "name": "QA Engineer",
        "prompt": None,
        "is_builtin": True,
    },
    {
        "name": "Orchestrator",
        "prompt": None,
        "is_builtin": True,
    },
]


@pytest.mark.asyncio
async def test_list_agent_presets_returns_builtins(store):
    """Built-in presets should always be available even with no user data."""
    presets = await store.list_agent_presets()
    names = [p["name"] for p in presets]
    assert "Lead Developer" in names
    assert "QA Engineer" in names
    assert "Orchestrator" in names


@pytest.mark.asyncio
async def test_builtin_presets_have_prompts(store):
    """Each built-in preset should have a non-empty prompt."""
    presets = await store.list_agent_presets()
    builtins = [p for p in presets if p["is_builtin"]]
    assert len(builtins) >= 3
    for preset in builtins:
        assert preset["prompt"], f"Built-in preset '{preset['name']}' has no prompt"
        assert len(preset["prompt"]) > 20, f"Built-in preset '{preset['name']}' prompt is too short"


@pytest.mark.asyncio
async def test_builtin_presets_are_not_deletable(store):
    """Attempting to delete a built-in preset should raise or return an error."""
    presets = await store.list_agent_presets()
    builtin = next(p for p in presets if p["is_builtin"])
    with pytest.raises(Exception):
        await store.delete_agent_preset(builtin["id"])


@pytest.mark.asyncio
async def test_builtin_presets_are_not_editable(store):
    """Built-in presets should not be modifiable by users."""
    presets = await store.list_agent_presets()
    builtin = next(p for p in presets if p["is_builtin"])
    with pytest.raises(Exception):
        await store.update_agent_preset(builtin["id"], name="Hacked")


# ── Custom Agent Presets (CRUD) ────────────────────────────────────────────


@pytest.mark.asyncio
async def test_create_custom_preset(store):
    """Users should be able to create custom agent presets."""
    preset = await store.create_agent_preset(
        name="Security Auditor",
        prompt="You are a security auditor. Review code for vulnerabilities and suggest fixes.",
    )
    assert preset["id"] is not None
    assert preset["name"] == "Security Auditor"
    assert preset["is_builtin"] is False
    assert "security auditor" in preset["prompt"].lower()


@pytest.mark.asyncio
async def test_create_preset_appears_in_list(store):
    """Created presets should appear in the list alongside builtins."""
    await store.create_agent_preset(
        name="DevOps Engineer",
        prompt="You manage infrastructure and CI/CD pipelines.",
    )
    presets = await store.list_agent_presets()
    names = [p["name"] for p in presets]
    # Should have builtins + custom
    assert "Lead Developer" in names
    assert "DevOps Engineer" in names


@pytest.mark.asyncio
async def test_create_preset_duplicate_name_fails(store):
    """Creating a preset with a name that already exists should fail."""
    await store.create_agent_preset(name="Tester", prompt="Run tests.")
    with pytest.raises(Exception):
        await store.create_agent_preset(name="Tester", prompt="Different prompt.")


@pytest.mark.asyncio
async def test_create_preset_empty_name_fails(store):
    """Preset name must not be empty."""
    with pytest.raises(Exception):
        await store.create_agent_preset(name="", prompt="Some prompt.")


@pytest.mark.asyncio
async def test_create_preset_empty_prompt_allowed(store):
    """Presets can have an empty prompt (user fills in at launch time)."""
    preset = await store.create_agent_preset(name="Blank Agent", prompt="")
    assert preset["name"] == "Blank Agent"
    assert preset["prompt"] == ""


@pytest.mark.asyncio
async def test_update_custom_preset(store):
    """Users should be able to update their custom presets."""
    preset = await store.create_agent_preset(
        name="Frontend Dev",
        prompt="You work on React components.",
    )
    await store.update_agent_preset(
        preset["id"],
        name="Frontend Developer",
        prompt="You are a senior frontend developer specializing in React and TypeScript.",
    )
    presets = await store.list_agent_presets()
    updated = next(p for p in presets if p["id"] == preset["id"])
    assert updated["name"] == "Frontend Developer"
    assert "TypeScript" in updated["prompt"]


@pytest.mark.asyncio
async def test_delete_custom_preset(store):
    """Users should be able to delete their custom presets."""
    preset = await store.create_agent_preset(
        name="Temp Agent",
        prompt="Temporary.",
    )
    await store.delete_agent_preset(preset["id"])
    presets = await store.list_agent_presets()
    names = [p["name"] for p in presets]
    assert "Temp Agent" not in names


@pytest.mark.asyncio
async def test_get_agent_preset_by_id(store):
    """Should be able to fetch a single preset by ID."""
    preset = await store.create_agent_preset(
        name="Backend Dev",
        prompt="You write Python backends.",
    )
    fetched = await store.get_agent_preset(preset["id"])
    assert fetched is not None
    assert fetched["name"] == "Backend Dev"
    assert fetched["prompt"] == "You write Python backends."


@pytest.mark.asyncio
async def test_get_nonexistent_preset_returns_none(store):
    """Fetching a preset that doesn't exist should return None."""
    result = await store.get_agent_preset(99999)
    assert result is None


@pytest.mark.asyncio
async def test_list_presets_builtins_first(store):
    """Built-in presets should appear before custom ones in the list."""
    await store.create_agent_preset(name="AAA Custom", prompt="First alphabetically.")
    presets = await store.list_agent_presets()
    # Find the boundary between builtins and customs
    builtin_indices = [i for i, p in enumerate(presets) if p["is_builtin"]]
    custom_indices = [i for i, p in enumerate(presets) if not p["is_builtin"]]
    if builtin_indices and custom_indices:
        assert max(builtin_indices) < min(custom_indices), \
            "Built-in presets should appear before custom presets"


# ── Team Presets ───────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_create_team_preset(store):
    """Users should be able to save a team configuration."""
    team = await store.create_team_preset(
        name="Default Trio",
        agent_names=["Lead Developer", "QA Engineer", "Orchestrator"],
    )
    assert team["id"] is not None
    assert team["name"] == "Default Trio"
    assert len(team["agents"]) == 3


@pytest.mark.asyncio
async def test_list_team_presets(store):
    """Should list all saved team presets."""
    await store.create_team_preset(
        name="Team Alpha",
        agent_names=["Lead Developer", "QA Engineer"],
    )
    await store.create_team_preset(
        name="Team Beta",
        agent_names=["Lead Developer", "Orchestrator"],
    )
    teams = await store.list_team_presets()
    names = [t["name"] for t in teams]
    assert "Team Alpha" in names
    assert "Team Beta" in names


@pytest.mark.asyncio
async def test_get_team_preset(store):
    """Fetching a team preset should include agent details."""
    team = await store.create_team_preset(
        name="Full Stack",
        agent_names=["Lead Developer", "QA Engineer", "Orchestrator"],
    )
    fetched = await store.get_team_preset(team["id"])
    assert fetched is not None
    assert fetched["name"] == "Full Stack"
    assert len(fetched["agents"]) == 3
    # Agents should have their full preset info (name + prompt)
    agent_names = [a["name"] for a in fetched["agents"]]
    assert "Lead Developer" in agent_names


@pytest.mark.asyncio
async def test_delete_team_preset(store):
    """Deleting a team preset should remove it from the list."""
    team = await store.create_team_preset(
        name="Disposable Team",
        agent_names=["Lead Developer"],
    )
    await store.delete_team_preset(team["id"])
    teams = await store.list_team_presets()
    names = [t["name"] for t in teams]
    assert "Disposable Team" not in names


@pytest.mark.asyncio
async def test_team_preset_duplicate_name_fails(store):
    """Team preset names must be unique."""
    await store.create_team_preset(
        name="My Team",
        agent_names=["Lead Developer"],
    )
    with pytest.raises(Exception):
        await store.create_team_preset(
            name="My Team",
            agent_names=["QA Engineer"],
        )


@pytest.mark.asyncio
async def test_team_preset_with_custom_agents(store):
    """Team presets should work with a mix of builtin and custom agent presets."""
    await store.create_agent_preset(
        name="Database Admin",
        prompt="You manage database schemas and migrations.",
    )
    team = await store.create_team_preset(
        name="Backend Team",
        agent_names=["Lead Developer", "Database Admin"],
    )
    fetched = await store.get_team_preset(team["id"])
    agent_names = [a["name"] for a in fetched["agents"]]
    assert "Lead Developer" in agent_names
    assert "Database Admin" in agent_names


# ── Default Team for Launch Modal ──────────────────────────────────────────


@pytest.mark.asyncio
async def test_get_default_team_agents(store):
    """Should provide the 3 default agents for the team launch modal."""
    defaults = await store.get_default_team_agents()
    assert len(defaults) == 3
    names = [a["name"] for a in defaults]
    assert "Lead Developer" in names
    assert "QA Engineer" in names
    assert "Orchestrator" in names
    # Each should have a prompt
    for agent in defaults:
        assert agent["prompt"], f"Default agent '{agent['name']}' has no prompt"


# ── Edge Cases ─────────────────────────────────────────────────────────────


@pytest.mark.asyncio
async def test_preset_name_with_special_characters(store):
    """Preset names with special characters should be stored safely."""
    preset = await store.create_agent_preset(
        name='Agent "O\'Brien"',
        prompt="Handle edge cases with <special> & characters.",
    )
    fetched = await store.get_agent_preset(preset["id"])
    assert fetched["name"] == 'Agent "O\'Brien"'
    assert "<special>" in fetched["prompt"]


@pytest.mark.asyncio
async def test_preset_long_prompt(store):
    """Presets should handle long prompts without truncation."""
    long_prompt = "You are a specialized agent. " * 500  # ~14KB
    preset = await store.create_agent_preset(
        name="Verbose Agent",
        prompt=long_prompt,
    )
    fetched = await store.get_agent_preset(preset["id"])
    assert fetched["prompt"] == long_prompt


@pytest.mark.asyncio
async def test_delete_preset_used_in_team(store):
    """Deleting an agent preset that's referenced by a team should handle gracefully."""
    await store.create_agent_preset(name="Temp Role", prompt="Temp.")
    await store.create_team_preset(
        name="Team With Temp",
        agent_names=["Lead Developer", "Temp Role"],
    )
    # Deleting the preset should work (team will have a dangling reference)
    presets = await store.list_agent_presets()
    temp = next(p for p in presets if p["name"] == "Temp Role")
    await store.delete_agent_preset(temp["id"])
    # Team should still be loadable (skip missing agents or show placeholder)
    team = await store.get_team_preset(
        (await store.list_team_presets())[0]["id"]
    )
    # Should not crash; missing agent is either skipped or has placeholder
    assert team is not None
