import { describe, it, expect } from "vitest";
import { ensureArray, ensureRecord, ensureString } from "../responseGuards";

describe("ensureArray", () => {
  it("returns the array when given an array", () => {
    expect(ensureArray([1, 2, 3])).toEqual([1, 2, 3]);
  });

  it("returns empty array for non-array values", () => {
    expect(ensureArray(null)).toEqual([]);
    expect(ensureArray(undefined)).toEqual([]);
    expect(ensureArray("string")).toEqual([]);
    expect(ensureArray(42)).toEqual([]);
    expect(ensureArray({})).toEqual([]);
  });
});

describe("ensureRecord", () => {
  it("returns the object when given a plain object", () => {
    const obj = { key: "value" };
    expect(ensureRecord(obj)).toEqual(obj);
  });

  it("returns null for non-object values", () => {
    expect(ensureRecord(null)).toBeNull();
    expect(ensureRecord(undefined)).toBeNull();
    expect(ensureRecord("string")).toBeNull();
    expect(ensureRecord(42)).toBeNull();
  });

  it("returns null for arrays", () => {
    expect(ensureRecord([1, 2])).toBeNull();
  });
});

describe("ensureString", () => {
  it("returns the string when given a string", () => {
    expect(ensureString("hello")).toBe("hello");
    expect(ensureString("")).toBe("");
  });

  it("returns empty string for non-string values", () => {
    expect(ensureString(null)).toBe("");
    expect(ensureString(undefined)).toBe("");
    expect(ensureString(42)).toBe("");
    expect(ensureString({})).toBe("");
    expect(ensureString([])).toBe("");
  });
});
