import { useEffect, useRef } from "react";

export function AutoTextarea({ minRows = 2, ...props }) {
  const ref = useRef(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = el.scrollHeight + "px";
  }, [props.value]);

  return (
    <textarea
      ref={ref}
      style={{ resize: "none", overflow: "hidden", minHeight: `${minRows * 1.5}rem` }}
      {...props}
    />
  );
}
