#!/usr/bin/env bash
# Sessions API smoke contracts; invoked after main safety/cleanup setup.

smoke_check_sessions() {
  # 1) health (public)
  body=""
  status=""
  run_request body status GET "$API_BASE/healthz" "" 0
  assert_equal "GET /healthz" "200" "$status"
  smoke_print_verbose_json "body" "$body"

  # 2) auth required check on protected endpoint
  body=""
  status=""
  run_request body status POST "$API_BASE/terminal/sessions" '{"actor_id":"owner","target":"'"$SMOKE_COMMAND_TARGET_ID"'","mode":"interactive"}' 0
  assert_equal "POST /terminal/sessions without auth rejected" "401" "$status"

  # 3) create session
  body=""
  status=""
  terminal_target="$SMOKE_COMMAND_TARGET_ID"
  if labtether_value_is_true "$ALLOW_REMOTE_EXEC"; then
    terminal_target="$REMOTE_EXEC_TARGET"
  fi
  terminal_payload=$(jq -cn --arg target "$terminal_target" '{actor_id:"owner",target:$target,mode:"interactive"}')
  run_request body status POST "$API_BASE/terminal/sessions" "$terminal_payload"
  assert_equal "POST /terminal/sessions" "201" "$status"
  smoke_print_verbose_json "session response" "$body"
  session_id=$(extract_json_string "id" "$body")
  if [[ -z "$session_id" ]]; then
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse session id from response\n'
    session_id=""
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created session %s\n' "$session_id"
    smoke_register_cleanup DELETE "/terminal/sessions/${session_id}" "terminal session ${session_id}"
  fi

  # 4) create command
  if [[ -n "$session_id" ]]; then
    body=""
    status=""
    run_request body status POST "$API_BASE/terminal/sessions/${session_id}/commands" '{"actor_id":"owner","command":"uname -a"}'
    assert_equal "POST /terminal/sessions/{id}/commands" "202" "$status"
    smoke_print_verbose_json "command response" "$body"

    command_id=$(extract_json_string "id" "$body")
    if [[ -z "$command_id" ]]; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] could not parse command id\n'
    else
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] command id parsed %s\n' "$command_id"
    fi

    # 5) list commands for session (auth)
    body=""
    status=""
    run_request body status GET "$API_BASE/terminal/sessions/${session_id}/commands"
    assert_equal "GET /terminal/sessions/{id}/commands" "200" "$status"
    if [[ -n "$command_id" && "$body" == *"$command_id"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] command id found in session command list\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] command id not found in session command list\n'
    fi

    # 6) recent commands endpoint (auth)
    body=""
    status=""
    run_request body status GET "$API_BASE/terminal/commands/recent?limit=12"
    assert_equal "GET /terminal/commands/recent" "200" "$status"
    if [[ "$body" == *"$session_id"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] recent commands includes session id\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] recent commands does not include session id\n'
    fi

    # 7) audit events
    body=""
    status=""
    run_request body status GET "$API_BASE/audit/events?limit=20"
    assert_equal "GET /audit/events" "200" "$status"
    if [[ -n "$body" && "$body" == *"terminal.command.queued"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] audit contains queued event\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] audit did not include terminal.command.queued\n'
    fi

    # 8) worker stats
    body=""
    status=""
    run_request body status GET "$API_BASE/worker/stats"  '' 0
    assert_not_equal "GET /worker/stats" "000" "$status"
    if [[ "$status" == "200" ]]; then
      if contains "processed_jobs" "$body"; then
        PASS_COUNT=$((PASS_COUNT + 1))
        printf '  [PASS] worker stats payload returned\n'
      else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] worker stats missing processed_jobs\n'
      fi
    fi
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] Skipping command flow because session id missing\n'
  fi

  # 9) connector list and agent health checks
  body=""
  status=""
  run_request body status GET "$API_BASE/connectors"
  assert_equal "GET /connectors" "200" "$status"

  if [[ "$SKIP_COMPOSE" == "0" && "${AGENT_AVAILABLE:-0}" == "1" ]]; then
    body=""
    status=""
    run_request body status GET "$AGENT_BASE/healthz" "" 0
    assert_equal "GET /agent/healthz" "200" "$status"
  elif [[ "$SKIP_COMPOSE" == "0" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] GET /agent/healthz skipped (agent not reachable during startup)\n'
  else
    if check_http_status "$AGENT_BASE/healthz" 200 2; then
      body=""
      status=""
      run_request body status GET "$AGENT_BASE/healthz" "" 0
      assert_equal "GET /agent/healthz" "200" "$status"
    else
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] GET /agent/healthz skipped (non-compose mode; agent service not running)\n'
    fi
  fi
}
