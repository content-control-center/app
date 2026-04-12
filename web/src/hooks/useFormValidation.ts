import { useCallback, useState } from "react";
import { z } from "zod";

import { type FieldErrors } from "@/lib";

/**
 * Generic form state + validation hook driven by a zod object schema.
 *
 * Responsibilities:
 *  - Owns the form's `values` state.
 *  - `setField(name, value)` writes the field; once the form has been submitted
 *    at least once, it also re-validates that single field so the user sees
 *    errors clear as they type.
 *  - `validate()` runs the full schema once (called on submit). On failure it
 *    populates `fieldErrors` and returns `undefined`; on success it returns the
 *    parsed data.
 *
 * Intentionally minimal — no dirty/touched tracking, no async validation.
 * Add those if a form actually needs them.
 */
export function useFormValidation<T extends z.ZodObject<z.ZodRawShape>>(
  schema: T,
  defaultValues: z.infer<T>
) {
  type Values = z.infer<T>;

  const [values, setValues] = useState<Values>(defaultValues);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors<Values>>({});
  const [hasSubmitted, setHasSubmitted] = useState(false);

  const setField = useCallback(
    <K extends keyof Values>(name: K, value: Values[K]) => {
      setValues((prev) => {
        const next = { ...prev, [name]: value } as Values;
        if (hasSubmitted) {
          const result = schema.safeParse(next);
          if (result.success) {
            setFieldErrors({});
          } else {
            const fieldIssue = result.error.issues.find((issue) => issue.path[0] === name);
            setFieldErrors((prevErrors) => ({
              ...prevErrors,
              [name]: fieldIssue?.message,
            }));
          }
        }
        return next;
      });
    },
    [schema, hasSubmitted]
  );

  const validate = useCallback((): Values | undefined => {
    setHasSubmitted(true);
    const result = schema.safeParse(values);
    if (result.success) {
      setFieldErrors({});
      return result.data as Values;
    }
    const errors: FieldErrors<Values> = {};
    for (const issue of result.error.issues) {
      const key = issue.path[0];
      if (typeof key === "string" && !(key in errors)) {
        (errors as Record<string, string>)[key] = issue.message;
      }
    }
    setFieldErrors(errors);
    return undefined;
  }, [schema, values]);

  const reset = useCallback(() => {
    setValues(defaultValues);
    setFieldErrors({});
    setHasSubmitted(false);
  }, [defaultValues]);

  return { values, setField, fieldErrors, validate, reset };
}
