"use client";
// App-specific composites built on the shadcn primitives. Keeps page code lean
// and the visual language consistent (status colors, copy/secret, confirm flow).
import * as React from "react";
import { Check, Copy, Eye, EyeOff, Inbox, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

// ---------- PageHeader ----------
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

// ---------- Stat ----------
export function Stat({
  label,
  value,
  sub,
}: {
  label: React.ReactNode;
  value: React.ReactNode;
  sub?: React.ReactNode;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tracking-tight">{value}</div>
        {sub && <p className="mt-1 text-xs text-muted-foreground">{sub}</p>}
      </CardContent>
    </Card>
  );
}

// ---------- EmptyState ----------
export function EmptyState({
  title,
  hint,
  action,
  icon: Icon = Inbox,
}: {
  title: React.ReactNode;
  hint?: React.ReactNode;
  action?: React.ReactNode;
  icon?: LucideIcon;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed px-6 py-16 text-center">
      <Icon className="mb-3 h-8 w-8 text-muted-foreground" />
      <p className="font-medium">{title}</p>
      {hint && (
        <p className="mt-1 max-w-md text-sm text-muted-foreground">{hint}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

// ---------- StatusBadge ----------
type BadgeVariant =
  | "success"
  | "warning"
  | "destructive"
  | "info"
  | "secondary";

const statusVariant: Record<string, BadgeVariant> = {
  online: "success",
  ready: "success",
  verified: "success",
  active: "success",
  issued: "success",
  scanning: "warning",
  converting: "warning",
  pending: "warning",
  uploading: "warning",
  expiring: "warning",
  renewing: "warning",
  uploaded: "info",
  offline: "secondary",
  expired: "secondary",
  rejected: "destructive",
  failed: "destructive",
  revoked: "destructive",
  frozen: "destructive",
};

export function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant={statusVariant[status] ?? "secondary"} className="capitalize">
      {status}
    </Badge>
  );
}

// ---------- Copy ----------
export function CopyButton({
  value,
  className,
  label = true,
}: {
  value: string;
  className?: string;
  label?: boolean;
}) {
  const [copied, setCopied] = React.useState(false);
  return (
    <Button
      type="button"
      variant="outline"
      size={label ? "sm" : "icon"}
      className={className}
      onClick={() => {
        navigator.clipboard?.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? (
        <Check className="h-3.5 w-3.5" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
      {label && (copied ? "Copied" : "Copy")}
    </Button>
  );
}

export function CopyField({
  value,
  mono = true,
}: {
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      <code
        className={cn(
          "flex-1 overflow-x-auto whitespace-nowrap rounded-md border bg-muted px-3 py-2 text-sm",
          mono && "font-mono",
        )}
      >
        {value}
      </code>
      <CopyButton value={value} />
    </div>
  );
}

// ---------- CodeBlock ----------
export function CodeBlock({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  return (
    <div className="group relative">
      <pre
        className={cn(
          "overflow-x-auto rounded-lg border bg-muted p-4 font-mono text-sm leading-relaxed",
          className,
        )}
      >
        <code>{children}</code>
      </pre>
      <div className="absolute right-2 top-2 opacity-0 transition-opacity group-hover:opacity-100">
        <CopyButton value={children} label={false} />
      </div>
    </div>
  );
}

// ---------- SecretReveal (one-time secret) ----------
export function SecretReveal({ value }: { value: string }) {
  const [shown, setShown] = React.useState(false);
  return (
    <div className="space-y-2">
      <Alert>
        <AlertTitle>Shown once</AlertTitle>
        <AlertDescription>
          Store this secret securely — it won&apos;t be shown again. Don&apos;t
          paste it into public channels.
        </AlertDescription>
      </Alert>
      <div className="flex items-center gap-2">
        <code className="flex-1 overflow-x-auto whitespace-nowrap rounded-md border bg-muted px-3 py-2 font-mono text-sm">
          {shown ? value : "•".repeat(Math.min(44, value.length))}
        </code>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setShown((s) => !s)}
        >
          {shown ? (
            <EyeOff className="h-3.5 w-3.5" />
          ) : (
            <Eye className="h-3.5 w-3.5" />
          )}
          {shown ? "Hide" : "Reveal"}
        </Button>
        <CopyButton value={value} />
      </div>
    </div>
  );
}

// ---------- ConfirmDialog (controlled) ----------
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = "Confirm",
  confirmWord,
  destructive,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  confirmLabel?: string;
  confirmWord?: string;
  destructive?: boolean;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = React.useState("");
  const ready = !confirmWord || typed === confirmWord;
  React.useEffect(() => {
    if (!open) setTyped("");
  }, [open]);
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          )}
        </AlertDialogHeader>
        {confirmWord && (
          <div className="space-y-1.5">
            <p className="text-sm text-muted-foreground">
              Type{" "}
              <span className="font-mono font-medium text-foreground">
                {confirmWord}
              </span>{" "}
              to confirm
            </p>
            <Input
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoFocus
            />
          </div>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={!ready}
            onClick={onConfirm}
            className={cn(
              destructive &&
                "bg-destructive text-destructive-foreground hover:bg-destructive/90",
            )}
          >
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
