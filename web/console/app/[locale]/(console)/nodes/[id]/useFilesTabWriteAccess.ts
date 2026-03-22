"use client";

import { useCallback, useEffect, useState, type RefObject } from "react";

type UseFilesTabWriteAccessArgs = {
  currentPath: string;
  uploadInputRef: RefObject<HTMLInputElement | null>;
};

export function useFilesTabWriteAccess({
  currentPath,
  uploadInputRef,
}: UseFilesTabWriteAccessArgs) {
  const [writeEnabled, setWriteEnabled] = useState(false);
  const [uploadDestinationPath, setUploadDestinationPath] = useState("~");
  const [enableWriteOpen, setEnableWriteOpen] = useState(false);

  useEffect(() => {
    setUploadDestinationPath(currentPath);
  }, [currentPath]);

  const enableWriteActions = useCallback(() => {
    if (writeEnabled) {
      setWriteEnabled(false);
      return;
    }
    setEnableWriteOpen(true);
  }, [writeEnabled]);

  const handleConfirmEnableWrite = useCallback(() => {
    setWriteEnabled(true);
    setEnableWriteOpen(false);
  }, []);

  const handleUploadHere = useCallback((targetDirPath: string) => {
    setUploadDestinationPath(targetDirPath);
    uploadInputRef.current?.click();
  }, [uploadInputRef]);

  return {
    writeEnabled,
    enableWriteOpen,
    setEnableWriteOpen,
    uploadDestinationPath,
    enableWriteActions,
    handleConfirmEnableWrite,
    handleUploadHere,
  };
}
