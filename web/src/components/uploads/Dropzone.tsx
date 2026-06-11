import { useRef, useState } from "react";
import { UploadSimpleIcon } from "@phosphor-icons/react";
import { cn } from "@/lib";
import { UPLOAD_ACCEPT, UPLOAD_LIMITS_LABEL } from "@/lib/assetStatus";

type Props = {
  onFiles: (files: File[]) => void;
  className?: string;
};

/** Click-to-browse + native drag-and-drop target for .md / .pdf files. */
export function Dropzone({ onFiles, className }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const open = () => inputRef.current?.click();

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={open}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          open();
        }
      }}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        const files = Array.from(e.dataTransfer.files);
        if (files.length) onFiles(files);
      }}
      className={cn(
        "flex flex-col items-center justify-center gap-2 px-6 py-10 text-center cursor-pointer transition-colors outline-none",
        dragging ? "bg-quaternary" : "bg-tertiary hover:bg-quaternary",
        className,
      )}
    >
      <UploadSimpleIcon className="size-6 text-tertiary-foreground" />
      <p className="text-sm text-foreground">Drop files here or click to browse</p>
      <p className="text-xs text-tertiary-foreground">{UPLOAD_LIMITS_LABEL}</p>
      <input
        ref={inputRef}
        type="file"
        accept={UPLOAD_ACCEPT}
        multiple
        hidden
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          if (files.length) onFiles(files);
          e.target.value = "";
        }}
      />
    </div>
  );
}
