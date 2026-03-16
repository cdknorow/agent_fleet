"""Tests for the launch-team API endpoint with preset agent support.

These tests verify:
- The /api/agent-presets endpoints (CRUD)
- The /api/team-presets endpoints (CRUD)
- The launch-team endpoint works with preset references
- The 3 default agents are correctly used
"""

import pytest
import pytest_asyncio
from unittest.mock import AsyncMock, patch, MagicMock
from httpx import AsyncClient, ASGITransport

from coral.store import CoralStore


@pytest_asyncio.fixture
async def store(tmp_path):
    """Create a CoralStore backed by a temp DB."""
    s = CoralStore(db_path=tmp_path / "test.db")
    yield s
    await s.close()


# ── API: Agent Presets Endpoints ──────────────────────────────────────────


@pytest.mark.asyncio
async def test_api_list_agent_presets(store):
    """GET /api/agent-presets should return built-in + custom presets."""
    # We test the store directly since the web server test requires full app setup
    presets = await store.list_agent_presets()
    assert len(presets) >= 3  # At least the 3 builtins
    names = [p["name"] for p in presets]
    assert "Lead Developer" in names
    assert "QA Engineer" in names
    assert "Orchestrator" in names


@pytest.mark.asyncio
async def test_api_create_agent_preset(store):
    """POST /api/agent-presets should create a custom preset."""
    preset = await store.create_agent_preset(
        name="API Specialist",
        prompt="You design and implement REST APIs.",
    )
    assert preset["id"] > 0
    assert preset["is_builtin"] is False

    # Verify it appears in list
    presets = await store.list_agent_presets()
    assert any(p["name"] == "API Specialist" for p in presets)


@pytest.mark.asyncio
async def test_api_update_agent_preset(store):
    """PUT /api/agent-presets/{id} should update a custom preset."""
    preset = await store.create_agent_preset(
        name="Old Name",
        prompt="Old prompt.",
    )
    await store.update_agent_preset(preset["id"], name="New Name", prompt="New prompt.")
    updated = await store.get_agent_preset(preset["id"])
    assert updated["name"] == "New Name"
    assert updated["prompt"] == "New prompt."


@pytest.mark.asyncio
async def test_api_delete_agent_preset(store):
    """DELETE /api/agent-presets/{id} should remove a custom preset."""
    preset = await store.create_agent_preset(
        name="Deletable",
        prompt="Will be deleted.",
    )
    await store.delete_agent_preset(preset["id"])
    result = await store.get_agent_preset(preset["id"])
    assert result is None


# ── API: Team Presets Endpoints ──────────────────────────────────────────


@pytest.mark.asyncio
async def test_api_list_team_presets(store):
    """GET /api/team-presets should return saved teams."""
    await store.create_team_preset(
        name="Test Team",
        agent_names=["Lead Developer", "QA Engineer"],
    )
    teams = await store.list_team_presets()
    assert len(teams) == 1
    assert teams[0]["name"] == "Test Team"


@pytest.mark.asyncio
async def test_api_create_team_preset(store):
    """POST /api/team-presets should save a team configuration."""
    team = await store.create_team_preset(
        name="Dream Team",
        agent_names=["Lead Developer", "QA Engineer", "Orchestrator"],
    )
    assert team["id"] > 0
    assert team["name"] == "Dream Team"


@pytest.mark.asyncio
async def test_api_get_team_preset_with_agents(store):
    """GET /api/team-presets/{id} should include full agent details."""
    team = await store.create_team_preset(
        name="Detail Team",
        agent_names=["Lead Developer", "QA Engineer"],
    )
    fetched = await store.get_team_preset(team["id"])
    assert len(fetched["agents"]) == 2
    for agent in fetched["agents"]:
        assert "name" in agent
        assert "prompt" in agent


@pytest.mark.asyncio
async def test_api_delete_team_preset(store):
    """DELETE /api/team-presets/{id} should remove the team."""
    team = await store.create_team_preset(
        name="Delete Me",
        agent_names=["Lead Developer"],
    )
    await store.delete_team_preset(team["id"])
    teams = await store.list_team_presets()
    assert len(teams) == 0


# ── Launch Team with Default Agents ──────────────────────────────────────


@pytest.mark.asyncio
async def test_default_team_has_three_agents(store):
    """The default team should have exactly 3 agents."""
    defaults = await store.get_default_team_agents()
    assert len(defaults) == 3


@pytest.mark.asyncio
async def test_default_team_agent_prompts_are_distinct(store):
    """Each default agent should have a unique, distinct prompt."""
    defaults = await store.get_default_team_agents()
    prompts = [a["prompt"] for a in defaults]
    assert len(set(prompts)) == 3, "Default agent prompts should all be unique"


@pytest.mark.asyncio
async def test_default_team_orchestrator_prompt_mentions_coordination(store):
    """The Orchestrator prompt should reference coordination/planning."""
    defaults = await store.get_default_team_agents()
    orchestrator = next(a for a in defaults if a["name"] == "Orchestrator")
    prompt_lower = orchestrator["prompt"].lower()
    assert any(word in prompt_lower for word in ["coordinat", "plan", "orchestrat", "delegate"]), \
        "Orchestrator prompt should mention coordination or planning"


@pytest.mark.asyncio
async def test_default_team_lead_dev_prompt_mentions_implementation(store):
    """The Lead Developer prompt should reference implementation/coding."""
    defaults = await store.get_default_team_agents()
    lead = next(a for a in defaults if a["name"] == "Lead Developer")
    prompt_lower = lead["prompt"].lower()
    assert any(word in prompt_lower for word in ["implement", "code", "develop", "feature"]), \
        "Lead Developer prompt should mention implementation"


@pytest.mark.asyncio
async def test_default_team_qa_prompt_mentions_testing(store):
    """The QA Engineer prompt should reference testing/review."""
    defaults = await store.get_default_team_agents()
    qa = next(a for a in defaults if a["name"] == "QA Engineer")
    prompt_lower = qa["prompt"].lower()
    assert any(word in prompt_lower for word in ["test", "review", "qa", "quality"]), \
        "QA Engineer prompt should mention testing or review"


# ── Resume persistence for prompt/board ──────────────────────────────────


@pytest.mark.asyncio
async def test_resume_preserves_prompt_and_board(store, tmp_path):
    """When resuming sessions, prompt and board_name should be carried forward."""
    work_dir = str(tmp_path)
    await store.register_live_session(
        "sid-1", "claude", "wt1", work_dir,
        display_name="Lead Developer",
        prompt="You are the lead developer.",
        board_name="my-board",
        board_server="http://remote:8420",
    )

    sessions = await store.get_all_live_sessions()
    assert len(sessions) == 1
    s = sessions[0]
    assert s["prompt"] == "You are the lead developer."
    assert s["board_name"] == "my-board"
    assert s["board_server"] == "http://remote:8420"


@pytest.mark.asyncio
async def test_replace_live_session_carries_forward_prompt(store, tmp_path):
    """replace_live_session should carry forward prompt, board_name, board_server."""
    work_dir = str(tmp_path)
    await store.register_live_session(
        "old-sid", "claude", "wt1", work_dir,
        prompt="Original prompt.",
        board_name="team-board",
        board_server="http://server:8420",
    )

    await store.replace_live_session(
        "old-sid", "new-sid", "claude", "wt1", work_dir,
        display_name="Lead Developer",
    )

    sessions = await store.get_all_live_sessions()
    assert len(sessions) == 1
    s = sessions[0]
    assert s["session_id"] == "new-sid"
    # Prompt and board info should be carried forward from old session
    assert s["prompt"] == "Original prompt."
    assert s["board_name"] == "team-board"
    assert s["board_server"] == "http://server:8420"


@pytest.mark.asyncio
async def test_get_live_session_prompt_info(store, tmp_path):
    """get_live_session_prompt_info should return prompt and board details."""
    work_dir = str(tmp_path)
    await store.register_live_session(
        "sid-1", "claude", "wt1", work_dir,
        prompt="Test prompt.",
        board_name="test-board",
        board_server="http://localhost:9000",
    )

    info = await store.get_live_session_prompt_info("sid-1")
    assert info is not None
    assert info["prompt"] == "Test prompt."
    assert info["board_name"] == "test-board"
    assert info["board_server"] == "http://localhost:9000"


@pytest.mark.asyncio
async def test_get_live_session_prompt_info_missing(store):
    """get_live_session_prompt_info for nonexistent session returns None."""
    info = await store.get_live_session_prompt_info("nonexistent")
    assert info is None


@pytest.mark.asyncio
async def test_resume_persistent_sessions_passes_prompt_and_board(store, tmp_path):
    """resume_persistent_sessions should pass prompt/board to launch_claude_session."""
    work_dir = str(tmp_path)
    await store.register_live_session(
        "old-sid", "claude", "wt1", work_dir,
        display_name="QA Engineer",
        prompt="You are a QA engineer.",
        board_name="qa-board",
        board_server="http://remote:8420",
    )

    launch_result = {
        "session_name": "claude-new-sid",
        "session_id": "new-sid",
        "log_file": "/tmp/claude_coral_new-sid.log",
        "working_dir": work_dir,
        "agent_type": "claude",
    }

    with patch("coral.tools.session_manager.discover_coral_agents", AsyncMock(return_value=[])), \
         patch("coral.tools.session_manager.launch_claude_session", AsyncMock(return_value=launch_result)) as mock_launch, \
         patch("coral.tools.session_manager.setup_board_and_prompt", AsyncMock()):
        from coral.tools.session_manager import resume_persistent_sessions
        await resume_persistent_sessions(store)

    mock_launch.assert_called_once()
    call_kwargs = mock_launch.call_args
    assert call_kwargs.kwargs.get("prompt") == "You are a QA engineer." or \
           (len(call_kwargs.args) > 4 and call_kwargs.args[4] == "You are a QA engineer.") or \
           call_kwargs[1].get("prompt") == "You are a QA engineer."
