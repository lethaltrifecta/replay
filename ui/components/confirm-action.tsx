"use client";

import type { ReactNode } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "./ui/alert-dialog";
import { Button } from "@/components/ui/button";

export function ConfirmAction({
  title,
  description,
  confirmLabel,
  pendingLabel,
  onConfirm,
  disabled,
  pending,
  triggerLabel,
  triggerVariant = "default",
  children,
}: {
  title: string;
  description: string;
  confirmLabel: string;
  pendingLabel?: string;
  onConfirm: () => void | Promise<void>;
  disabled?: boolean;
  pending?: boolean;
  triggerLabel?: string;
  triggerVariant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
  children?: ReactNode;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        {children ?? (
          <Button variant={triggerVariant} disabled={disabled || pending}>
            {triggerLabel}
          </Button>
        )}
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction disabled={pending} onClick={() => void onConfirm()}>
            {pending ? pendingLabel ?? "Working..." : confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
