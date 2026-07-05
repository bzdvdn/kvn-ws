import { useState, useCallback, useRef, useEffect, type ReactNode } from "react";
import { colors, fontSize } from "./theme";

export type ValidationRule = {
  validate: (val: string) => string | undefined;
};

// @sk-task kvn-web-redesign#T3.1: validation rules for form fields (AC-006, AC-007)
export const rules = {
  required: (msg = "Required"): ValidationRule => ({
    validate: (val) => (val.trim() ? undefined : msg),
  }),
  wsUrl: (): ValidationRule => ({
    validate: (val) => {
      if (!val.trim()) return undefined;
      return /^wss?:\/\//.test(val) ? undefined : "Must start with ws:// or wss://";
    },
  }),
  positiveNumber: (msg = "Must be a positive number"): ValidationRule => ({
    validate: (val) => {
      if (!val.trim()) return undefined;
      const n = Number(val);
      return Number.isFinite(n) && n > 0 ? undefined : msg;
    },
  }),
  hexKey: (len: number, msg?: string): ValidationRule => ({
    validate: (val) => {
      if (!val.trim()) return undefined;
      const re = new RegExp(`^[0-9a-fA-F]{${len}}$`);
      return re.test(val) ? undefined : (msg || `Must be ${len} hex characters`);
    },
  }),
};

interface FormFieldProps {
  label: string;
  error?: string;
  children: ReactNode;
}

const fieldContainer: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 3,
};

const labelStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  fontSize: fontSize.sm,
  color: colors.textDim,
  textTransform: "uppercase",
  letterSpacing: "0.4px",
};

const errorStyle: React.CSSProperties = {
  fontSize: 10,
  color: colors.error,
  lineHeight: 1.3,
};

export default function FormField({ label, error, children }: FormFieldProps) {
  return (
    <div style={fieldContainer}>
      <span style={error ? { ...labelStyle, color: colors.error } : labelStyle}>
        {label}
      </span>
      {children}
      {error && <span style={errorStyle}>{error}</span>}
    </div>
  );
}

export function useFieldValidation() {
  const [errors, setErrors] = useState<Record<string, string>>({});
  const timers = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  const validate = useCallback((key: string, value: string, fieldRules: ValidationRule[], immediate = false) => {
    if (timers.current[key]) {
      clearTimeout(timers.current[key]);
    }

    const run = () => {
      for (const rule of fieldRules) {
        const err = rule.validate(value);
        if (err) {
          setErrors((prev) => ({ ...prev, [key]: err }));
          return;
        }
      }
      setErrors((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
    };

    if (immediate) {
      run();
    } else {
      timers.current[key] = setTimeout(run, 300);
    }
  }, []);

  const setError = useCallback((key: string, msg: string) => {
    setErrors((prev) => ({ ...prev, [key]: msg }));
  }, []);

  const clearError = useCallback((key: string) => {
    setErrors((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const isValid = Object.keys(errors).length === 0;

  useEffect(() => {
    const t = timers.current;
    return () => { Object.values(t).forEach(clearTimeout); };
  }, []);

  return { errors, validate, setError, clearError, isValid };
}
