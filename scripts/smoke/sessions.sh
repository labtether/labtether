#!/usr/bin/env bash
# Sessions API smoke contracts; invoked after main safety/cleanup setup.

smoke_wait_for_terminal_command() {
  local session_id=$1 command_id=$2
  local deadline=$((SECONDS + 30))
  local response_body="" response_status="" command_json="" command_status="" command_output=""

  while (( SECONDS < deadline )); do
    response_body=""
    response_status=""
    if ! run_request response_body response_status GET "$API_BASE/terminal/sessions/${session_id}/commands" "" 1 3; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] terminal command result request failed\n'
      return 1
    fi
    if [[ "$response_status" != "200" ]] || ! command_json=$(jq -ce --arg id "$command_id" '.commands[]? | select(.id == $id)' <<<"$response_body"); then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] terminal command result missing or invalid (HTTP %s)\n' "$response_status"
      return 1
    fi
    command_status=$(jq -r '.status // empty' <<<"$command_json")
    case "$command_status" in
      succeeded)
        command_output=$(jq -r '.output // empty' <<<"$command_json")
        if [[ "$command_output" =~ [^[:space:]] ]]; then
          PASS_COUNT=$((PASS_COUNT + 1))
          printf '  [PASS] terminal uname command completed with output\n'
          return 0
        fi
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] terminal uname command succeeded without output\n'
        return 1
        ;;
      queued|running) sleep 1 ;;
      failed|timed_out|timeout|cancelled|canceled|error)
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] terminal uname command ended with status %s\n' "$command_status"
        return 1
        ;;
      *)
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] terminal uname command has unknown status %s\n' "$command_status"
        return 1
        ;;
    esac
  done
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] terminal uname command did not finish within 30 seconds\n'
  return 1
}

smoke_check_queued_command_audit() {
  local session_id=$1 command_id=$2 since=$3
  local page_size=200 max_pages=10 deadline=$((SECONDS + 30))
  local offset=0 page=0 response_body="" response_status="" count="" oldest=""

  while (( page < max_pages && SECONDS < deadline )); do
    response_body=""
    response_status=""
    if ! run_request response_body response_status GET "$API_BASE/audit/events?limit=${page_size}&offset=${offset}" "" 1 3; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] audit event request failed\n'
      return 1
    fi
    if [[ "$response_status" != "200" ]] || ! jq -e '.events | type == "array"' <<<"$response_body" >/dev/null; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] audit event response invalid (HTTP %s)\n' "$response_status"
      return 1
    fi
    if jq -e --arg session "$session_id" --arg command "$command_id" \
      '.events | any(.[]; .type == "terminal.command.queued" and .session_id == $session and .command_id == $command)' \
      <<<"$response_body" >/dev/null; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] audit contains this terminal command queued event\n'
      return 0
    fi

    count=$(jq -r '.events | length' <<<"$response_body")
    if (( count < page_size )); then
      break
    fi
    oldest=$(jq -r '.events[-1].timestamp // "" | if type == "string" then .[0:19] else "" end' <<<"$response_body")
    if [[ ! "$oldest" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}$ ]]; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] audit event timestamp missing or invalid\n'
      return 1
    fi
    if [[ "$oldest" < "$since" ]]; then
      break
    fi
    page=$((page + 1))
    offset=$((offset + page_size))
  done

  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf '  [FAIL] queued audit event for this command not found within the bounded request window\n'
  return 1
}

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
    assert_equal "POST /terminal/sessions/{id}/commands queued" "202" "$status"
    smoke_print_verbose_json "command response" "$body"

    command_id=$(extract_json_string "id" "$body")
    command_audit_since=$(jq -r '.command.created_at // "" | if type == "string" then .[0:19] else "" end' <<<"$body" 2>/dev/null) || command_audit_since=""
    if [[ -z "$command_id" ]]; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] could not parse command id\n'
    else
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] command id parsed %s\n' "$command_id"
      if labtether_value_is_true "$ALLOW_REMOTE_EXEC"; then
        smoke_wait_for_terminal_command "$session_id" "$command_id" || true
      else
        smoke_skip "terminal command completion (safe default uses an unroutable fixture)"
      fi
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

    # 7) audit event for this command, across bounded newest-first pages
    if [[ -n "$command_id" ]]; then
      if [[ "$command_audit_since" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}$ ]]; then
        smoke_check_queued_command_audit "$session_id" "$command_id" "$command_audit_since" || true
      else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] terminal command response missing creation timestamp for audit check\n'
      fi
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
    smoke_skip "GET /agent/healthz (agent not reachable during startup)"
  else
    if check_http_status "$AGENT_BASE/healthz" 200 2; then
      body=""
      status=""
      run_request body status GET "$AGENT_BASE/healthz" "" 0
      assert_equal "GET /agent/healthz" "200" "$status"
    else
      smoke_skip "GET /agent/healthz (non-compose mode; agent service not running)"
    fi
  fi
}
