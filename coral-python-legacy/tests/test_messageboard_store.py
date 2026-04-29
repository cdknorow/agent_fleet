"""Tests for the message board store layer."""

import pytest
import pytest_asyncio

from coral.messageboard.store import MessageBoardStore


@pytest_asyncio.fixture
async def store(tmp_path):
    s = MessageBoardStore(db_path=tmp_path / "test_board.db")
    yield s
    await s.close()


@pytest.mark.asyncio
async def test_subscribe_creates_subscriber(store):
    result = await store.subscribe("proj1", "agent-1", "Backend Dev")
    assert result["project"] == "proj1"
    assert result["session_id"] == "agent-1"
    assert result["job_title"] == "Backend Dev"
    assert result["last_read_id"] == 0


@pytest.mark.asyncio
async def test_subscribe_upsert_preserves_cursor(store):
    await store.subscribe("proj1", "agent-1", "Backend Dev")
    # Post a message from another agent and read to advance cursor
    await store.subscribe("proj1", "agent-2", "Frontend Dev")
    await store.post_message("proj1", "agent-2", "hello")
    msgs = await store.read_messages("proj1", "agent-1")
    assert len(msgs) == 1

    # Re-subscribe with new title
    result = await store.subscribe("proj1", "agent-1", "Senior Backend Dev")
    assert result["job_title"] == "Senior Backend Dev"
    # Cursor should be preserved (not reset to 0)
    assert result["last_read_id"] > 0


@pytest.mark.asyncio
async def test_unsubscribe(store):
    await store.subscribe("proj1", "agent-1", "Dev")
    removed = await store.unsubscribe("proj1", "agent-1")
    assert removed is True

    # Unsubscribing non-existent returns False
    removed = await store.unsubscribe("proj1", "agent-1")
    assert removed is False


@pytest.mark.asyncio
async def test_list_subscribers(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")
    await store.subscribe("proj2", "agent-3", "QA")

    subs = await store.list_subscribers("proj1")
    assert len(subs) == 2
    session_ids = {s["session_id"] for s in subs}
    assert session_ids == {"agent-1", "agent-2"}


@pytest.mark.asyncio
async def test_post_message(store):
    msg = await store.post_message("proj1", "agent-1", "Found a bug")
    assert msg["id"] is not None
    assert msg["project"] == "proj1"
    assert msg["content"] == "Found a bug"


@pytest.mark.asyncio
async def test_read_messages_excludes_own_and_advances_cursor(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-1", "msg from 1")
    await store.post_message("proj1", "agent-2", "msg from 2")

    # agent-1 reads: should only see agent-2's message
    msgs = await store.read_messages("proj1", "agent-1")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "msg from 2"
    assert msgs[0]["job_title"] == "Frontend"

    # agent-2 reads: should only see agent-1's message
    msgs = await store.read_messages("proj1", "agent-2")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "msg from 1"


@pytest.mark.asyncio
async def test_read_messages_twice_returns_empty_on_second(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "hello")

    msgs1 = await store.read_messages("proj1", "agent-1")
    assert len(msgs1) == 1

    # Second read with no new messages
    msgs2 = await store.read_messages("proj1", "agent-1")
    assert len(msgs2) == 0


@pytest.mark.asyncio
async def test_read_messages_unsubscribed_returns_empty(store):
    msgs = await store.read_messages("proj1", "nonexistent")
    assert msgs == []


@pytest.mark.asyncio
async def test_list_projects(store):
    await store.subscribe("proj1", "agent-1", "Dev")
    await store.subscribe("proj2", "agent-2", "Dev")
    await store.post_message("proj1", "agent-1", "hello")
    await store.post_message("proj1", "agent-1", "world")

    projects = await store.list_projects()
    assert len(projects) == 2
    p1 = next(p for p in projects if p["project"] == "proj1")
    assert p1["subscriber_count"] == 1
    assert p1["message_count"] == 2


@pytest.mark.asyncio
async def test_get_webhook_targets(store):
    await store.subscribe("proj1", "agent-1", "Dev", webhook_url="http://example.com/hook")
    await store.subscribe("proj1", "agent-2", "Dev")
    await store.subscribe("proj1", "agent-3", "Dev", webhook_url="http://example.com/hook2")

    targets = await store.get_webhook_targets("proj1", "agent-1")
    assert len(targets) == 1
    assert targets[0]["session_id"] == "agent-3"


@pytest.mark.asyncio
async def test_check_unread_with_notify_all(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    # No messages yet
    count = await store.check_unread("proj1", "agent-1")
    assert count == 0

    # agent-2 posts messages with @notify-all
    await store.post_message("proj1", "agent-2", "@notify-all msg 1")
    await store.post_message("proj1", "agent-2", "@notify-all msg 2")

    # agent-1 should see 2 unread
    count = await store.check_unread("proj1", "agent-1")
    assert count == 2

    # agent-2 should see 0 (own messages excluded)
    count = await store.check_unread("proj1", "agent-2")
    assert count == 0


@pytest.mark.asyncio
async def test_check_unread_notify_all_variants(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "@notify_all underscore variant")
    await store.post_message("proj1", "agent-2", "@notifyall no separator")
    await store.post_message("proj1", "agent-2", "@all short form")

    count = await store.check_unread("proj1", "agent-1")
    assert count == 3


@pytest.mark.asyncio
async def test_check_unread_with_session_id_mention(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "@agent-1 need your help")
    count = await store.check_unread("proj1", "agent-1")
    assert count == 1


@pytest.mark.asyncio
async def test_check_unread_with_job_title_mention(store):
    await store.subscribe("proj1", "agent-1", "Backend Dev")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "@Backend Dev can you review this?")
    count = await store.check_unread("proj1", "agent-1")
    assert count == 1


@pytest.mark.asyncio
async def test_check_unread_ignores_unmentioned_messages(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    # Message without any @mention — should NOT trigger notification
    await store.post_message("proj1", "agent-2", "just a general update")
    count = await store.check_unread("proj1", "agent-1")
    assert count == 0


@pytest.mark.asyncio
async def test_check_unread_mention_case_insensitive(store):
    await store.subscribe("proj1", "agent-1", "Backend Dev")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "@backend dev please look")
    count = await store.check_unread("proj1", "agent-1")
    assert count == 1


@pytest.mark.asyncio
async def test_check_unread_does_not_advance_cursor(store):
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "@notify-all hello")

    # Check twice — count should stay the same (cursor not advanced)
    count1 = await store.check_unread("proj1", "agent-1")
    count2 = await store.check_unread("proj1", "agent-1")
    assert count1 == 1
    assert count2 == 1

    # After reading, check should return 0
    await store.read_messages("proj1", "agent-1")
    count3 = await store.check_unread("proj1", "agent-1")
    assert count3 == 0


@pytest.mark.asyncio
async def test_check_unread_not_subscribed(store):
    count = await store.check_unread("proj1", "nonexistent")
    assert count == 0


@pytest.mark.asyncio
async def test_get_subscription_returns_active(store):
    await store.subscribe("proj1", "agent-1", "Backend Dev")
    sub = await store.get_subscription("agent-1")
    assert sub is not None
    assert sub["project"] == "proj1"
    assert sub["job_title"] == "Backend Dev"
    assert sub["session_id"] == "agent-1"


@pytest.mark.asyncio
async def test_get_subscription_returns_none_for_unknown(store):
    sub = await store.get_subscription("nonexistent")
    assert sub is None


@pytest.mark.asyncio
async def test_delete_project(store):
    await store.subscribe("proj1", "agent-1", "Dev")
    await store.post_message("proj1", "agent-1", "hello")

    await store.delete_project("proj1")

    subs = await store.list_subscribers("proj1")
    assert len(subs) == 0
    projects = await store.list_projects()
    assert len(projects) == 0


@pytest.mark.asyncio
async def test_get_all_subscriptions(store):
    """get_all_subscriptions returns a dict keyed by session_id."""
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")
    await store.subscribe("proj2", "agent-3", "QA")

    subs = await store.get_all_subscriptions()
    assert len(subs) == 3
    assert "agent-1" in subs
    assert subs["agent-1"]["project"] == "proj1"
    assert subs["agent-1"]["job_title"] == "Backend"
    assert subs["agent-3"]["project"] == "proj2"


@pytest.mark.asyncio
async def test_get_all_subscriptions_empty(store):
    """get_all_subscriptions returns empty dict when no subscribers."""
    subs = await store.get_all_subscriptions()
    assert subs == {}


@pytest.mark.asyncio
async def test_list_messages_returns_all_including_own(store):
    """list_messages returns all messages (no cursor, includes sender's own)."""
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-1", "msg from 1")
    await store.post_message("proj1", "agent-2", "msg from 2")
    await store.post_message("proj1", "agent-1", "msg from 1 again")

    msgs = await store.list_messages("proj1")
    assert len(msgs) == 3
    assert msgs[0]["content"] == "msg from 1"
    assert msgs[1]["content"] == "msg from 2"
    assert msgs[2]["content"] == "msg from 1 again"
    # list_messages includes job_title via JOIN
    assert msgs[0]["job_title"] == "Backend"
    assert msgs[1]["job_title"] == "Frontend"


@pytest.mark.asyncio
async def test_list_messages_does_not_advance_cursor(store):
    """list_messages should NOT advance the read cursor (side-effect-free)."""
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    await store.post_message("proj1", "agent-2", "hello")

    # Call list_messages (should not affect cursors)
    msgs = await store.list_messages("proj1")
    assert len(msgs) == 1

    # read_messages should still see the message (cursor not advanced)
    msgs = await store.read_messages("proj1", "agent-1")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "hello"


@pytest.mark.asyncio
async def test_list_messages_respects_limit(store):
    """list_messages respects the limit parameter."""
    await store.subscribe("proj1", "agent-1", "Dev")
    for i in range(10):
        await store.post_message("proj1", "agent-1", f"msg {i}")

    msgs = await store.list_messages("proj1", limit=3)
    assert len(msgs) == 3
    assert msgs[0]["content"] == "msg 0"


@pytest.mark.asyncio
async def test_list_messages_empty_project(store):
    """list_messages returns empty list for project with no messages."""
    msgs = await store.list_messages("nonexistent")
    assert msgs == []


@pytest.mark.asyncio
async def test_subscribe_upsert_does_not_reset_cursor(store):
    """Re-subscribing with same session_id should NOT reset last_read_id."""
    await store.subscribe("proj1", "agent-1", "Backend")
    await store.subscribe("proj1", "agent-2", "Frontend")

    # agent-2 posts, agent-1 reads (advancing cursor)
    await store.post_message("proj1", "agent-2", "hello")
    await store.read_messages("proj1", "agent-1")

    # Re-subscribe agent-1 with different title
    sub = await store.subscribe("proj1", "agent-1", "Senior Backend")
    assert sub["job_title"] == "Senior Backend"
    assert sub["last_read_id"] > 0

    # agent-2 posts again
    await store.post_message("proj1", "agent-2", "world")

    # agent-1 should only see the new message, not "hello" again
    msgs = await store.read_messages("proj1", "agent-1")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "world"


@pytest.mark.asyncio
async def test_read_messages_multiple_agents_independent_cursors(store):
    """Each agent has its own independent read cursor."""
    await store.subscribe("proj1", "a1", "Dev1")
    await store.subscribe("proj1", "a2", "Dev2")
    await store.subscribe("proj1", "a3", "Dev3")

    await store.post_message("proj1", "a1", "from a1")
    await store.post_message("proj1", "a2", "from a2")

    # a3 reads — sees messages from both a1 and a2
    msgs = await store.read_messages("proj1", "a3")
    assert len(msgs) == 2

    # a3 reads again — empty (cursor advanced)
    msgs = await store.read_messages("proj1", "a3")
    assert len(msgs) == 0

    # a1 reads — only sees a2's message (own excluded)
    msgs = await store.read_messages("proj1", "a1")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "from a2"

    # New message from a3
    await store.post_message("proj1", "a3", "from a3")

    # a1 reads again — sees only a3's message (a2's already read)
    msgs = await store.read_messages("proj1", "a1")
    assert len(msgs) == 1
    assert msgs[0]["content"] == "from a3"
