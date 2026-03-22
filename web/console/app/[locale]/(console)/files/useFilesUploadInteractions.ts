"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type React from "react";

type UseFilesUploadInteractionsArgs = {
  currentPath: string;
  uploadFile: (file: File, destPath: string) => Promise<void>;
  setDragOver: (dragOver: boolean) => void;
};

export function useFilesUploadInteractions({
  currentPath,
  uploadFile,
  setDragOver,
}: UseFilesUploadInteractionsArgs) {
  const [uploadDestinationPath, setUploadDestinationPath] = useState("~");
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setUploadDestinationPath(currentPath);
  }, [currentPath]);

  const handleDrop = useCallback(
    async (event: React.DragEvent) => {
      event.preventDefault();
      setDragOver(false);
      const files = Array.from(event.dataTransfer.files);
      if (files.length > 1) {
        const totalSize = files.reduce((sum, file) => sum + file.size, 0);
        const sizeStr = totalSize > 1024 * 1024
          ? `${(totalSize / 1024 / 1024).toFixed(1)} MB`
          : `${(totalSize / 1024).toFixed(1)} KB`;
        if (!confirm(`Upload ${files.length} files (${sizeStr} total)?`)) {
          return;
        }
      }
      for (const file of files) {
        await uploadFile(file, currentPath);
      }
    },
    [currentPath, setDragOver, uploadFile],
  );

  const handleFileInput = useCallback(
    async (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files || []);
      if (files.length > 1) {
        const totalSize = files.reduce((sum, file) => sum + file.size, 0);
        const sizeStr = totalSize > 1024 * 1024
          ? `${(totalSize / 1024 / 1024).toFixed(1)} MB`
          : `${(totalSize / 1024).toFixed(1)} KB`;
        if (!confirm(`Upload ${files.length} files (${sizeStr} total)?`)) {
          event.target.value = "";
          return;
        }
      }
      for (const file of files) {
        await uploadFile(file, uploadDestinationPath);
      }
      event.target.value = "";
    },
    [uploadDestinationPath, uploadFile],
  );

  const handleToolbarUpload = useCallback(() => {
    setUploadDestinationPath(currentPath);
    fileInputRef.current?.click();
  }, [currentPath]);

  const handleContextUploadHere = useCallback((targetDirPath: string) => {
    setUploadDestinationPath(targetDirPath);
    fileInputRef.current?.click();
  }, []);

  return {
    fileInputRef,
    handleDrop,
    handleFileInput,
    handleToolbarUpload,
    handleContextUploadHere,
  };
}
