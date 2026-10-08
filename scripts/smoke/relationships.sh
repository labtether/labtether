#!/usr/bin/env bash
# Relationships API smoke contracts; invoked after main safety/cleanup setup.

smoke_check_relationships() {
  # ── Stream 3: Dependency Graph & Incident Asset Linking ──

  log "\nDependency graph smoke checks"

  # Ensure we have two assets for dependency testing
  body=""
  status=""
  run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"type\":\"host\",\"name\":\"Dep Source ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"status\":\"online\",\"platform\":\"linux\"}"
  assert_equal "POST /assets/heartbeat (dep src)" "202" "$status"
  if [[ "$status" == "202" ]]; then
    smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_SRC_ID}" "asset ${SMOKE_DEP_SRC_ID}"
  fi

  body=""
  status=""
  run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"${SMOKE_DEP_TGT_ID}\",\"type\":\"host\",\"name\":\"Dep Target ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"status\":\"online\",\"platform\":\"linux\"}"
  assert_equal "POST /assets/heartbeat (dep tgt)" "202" "$status"
  if [[ "$status" == "202" ]]; then
    smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_TGT_ID}" "asset ${SMOKE_DEP_TGT_ID}"
  fi

  body=""
  status=""
  run_request body status POST "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies" "{\"source_asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"target_asset_id\":\"${SMOKE_DEP_TGT_ID}\",\"relationship_type\":\"depends_on\",\"direction\":\"downstream\",\"criticality\":\"high\"}"
  assert_equal "POST /assets/{id}/dependencies" "201" "$status"
  smoke_dep_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_dep_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created dependency %s\n' "$smoke_dep_id"
    smoke_register_cleanup DELETE "/assets/${SMOKE_DEP_SRC_ID}/dependencies/${smoke_dep_id}" "dependency ${smoke_dep_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse dependency id\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies?limit=10"
  assert_equal "GET /assets/{id}/dependencies" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/blast-radius?max_depth=3"
  assert_equal "GET /assets/{id}/blast-radius" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/assets/${SMOKE_DEP_TGT_ID}/upstream?max_depth=3"
  assert_equal "GET /assets/{id}/upstream" "200" "$status"

  if [[ -n "$smoke_dep_id" ]]; then
    body=""
    status=""
    run_request body status DELETE "$API_BASE/assets/${SMOKE_DEP_SRC_ID}/dependencies/${smoke_dep_id}"
    assert_equal "DELETE /assets/{id}/dependencies/{depId}" "200" "$status"
  fi

  log "\nIncident asset linking smoke checks"

  body=""
  status=""
  run_request body status POST "$API_BASE/incidents" "{\"title\":\"Smoke Asset Link Test ${SMOKE_RUN_TOKEN}\",\"severity\":\"low\"}"
  assert_equal "POST /incidents (asset link)" "201" "$status"
  smoke_link_inc_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_link_inc_id" ]]; then
    smoke_register_cleanup DELETE "/incidents/${smoke_link_inc_id}" "incident ${smoke_link_inc_id}"
    body=""
    status=""
    run_request body status POST "$API_BASE/incidents/${smoke_link_inc_id}/link-asset" "{\"asset_id\":\"${SMOKE_DEP_SRC_ID}\",\"role\":\"primary\"}"
    assert_equal "POST /incidents/{id}/link-asset" "201" "$status"

    body=""
    status=""
    run_request body status GET "$API_BASE/incidents/${smoke_link_inc_id}/assets"
    assert_equal "GET /incidents/{id}/assets" "200" "$status"
    if [[ "$body" == *"${SMOKE_DEP_SRC_ID}"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] incident asset list includes linked asset\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] incident asset list missing linked asset\n'
    fi
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse incident id for asset linking\n'
  fi
}
