"use client";
import { useState, useCallback, useRef } from "react";

interface UndoOperation {
  type: string;
  undo: () => Promise<void>;
  redo: () => Promise<void>;
}

const MAX_STACK = 50;

export function useTopologyUndo() {
  const undoStack = useRef<UndoOperation[]>([]);
  const redoStack = useRef<UndoOperation[]>([]);
  const [canUndo, setCanUndo] = useState(false);
  const [canRedo, setCanRedo] = useState(false);

  const push = useCallback((op: UndoOperation) => {
    undoStack.current.push(op);
    if (undoStack.current.length > MAX_STACK) {
      undoStack.current.shift();
    }
    redoStack.current = []; // clear redo on new action
    setCanUndo(true);
    setCanRedo(false);
  }, []);

  const undo = useCallback(async () => {
    const op = undoStack.current.pop();
    if (!op) return;
    try {
      await op.undo();
      redoStack.current.push(op);
    } catch {
      // If undo fails, put it back
      undoStack.current.push(op);
    }
    setCanUndo(undoStack.current.length > 0);
    setCanRedo(redoStack.current.length > 0);
  }, []);

  const redo = useCallback(async () => {
    const op = redoStack.current.pop();
    if (!op) return;
    try {
      await op.redo();
      undoStack.current.push(op);
    } catch {
      redoStack.current.push(op);
    }
    setCanUndo(undoStack.current.length > 0);
    setCanRedo(redoStack.current.length > 0);
  }, []);

  return { push, undo, redo, canUndo, canRedo };
}
