import { useState } from "react";
import { ModalContainer } from "@/components/ui/modal";
import { Button } from "@/components/ui/button";
import { XIcon } from "@phosphor-icons/react";
import { Dropzone } from "./Dropzone";
import { useUploadStore } from "@/stores/uploadStore";
import {
  UPLOAD_LIMITS_LABEL,
  formatBytes,
  validateUploadFile,
} from "@/lib/assetStatus";

type Props = {
  isOpen: boolean;
  onClose: () => void;
};

/**
 * Modal entry point for uploads: shows the limits, a drop zone, and a staged
 * file list the user reviews before clicking Upload. Progress then continues
 * non-blocking in the UploadTracker, so the modal closes on submit.
 */
export function UploadModal({ isOpen, onClose }: Props) {
  const enqueue = useUploadStore((s) => s.enqueue);
  const [staged, setStaged] = useState<File[]>([]);

  const reset = () => setStaged([]);

  const close = () => {
    reset();
    onClose();
  };

  const addFiles = (files: File[]) =>
    setStaged((prev) => [...prev, ...files]);

  const removeStaged = (index: number) =>
    setStaged((prev) => prev.filter((_, i) => i !== index));

  const handleUpload = () => {
    if (staged.length === 0) return;
    enqueue(staged);
    close();
  };

  return (
    <ModalContainer
      isOpen={isOpen}
      onClose={close}
      title="Upload to content bank"
      size="large"
    >
      <div className="flex flex-col gap-4">
        <p className="text-sm text-tertiary-foreground">
          Upload Markdown or PDF files. {UPLOAD_LIMITS_LABEL}. PDFs are processed
          in the background after upload.
        </p>

        <Dropzone onFiles={addFiles} />

        {staged.length > 0 && (
          <ul className="flex flex-col gap-1">
            {staged.map((file, index) => {
              const validation = validateUploadFile(file);
              return (
                <li
                  key={`${file.name}-${index}`}
                  className="flex items-center justify-between gap-3 py-2"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm text-foreground">
                      {file.name}
                    </p>
                    {validation.ok ? (
                      <p className="text-xs text-tertiary-foreground">
                        {formatBytes(file.size)}
                      </p>
                    ) : (
                      <p className="text-xs text-destructive">
                        {validation.error}
                      </p>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="smIcon"
                    onClick={() => removeStaged(index)}
                    aria-label={`Remove ${file.name}`}
                  >
                    <XIcon className="size-4" />
                  </Button>
                </li>
              );
            })}
          </ul>
        )}

        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button onClick={handleUpload} disabled={staged.length === 0}>
            Upload{staged.length > 0 ? ` (${staged.length})` : ""}
          </Button>
        </div>
      </div>
    </ModalContainer>
  );
}
