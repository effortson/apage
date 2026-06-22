import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn merges class names, resolving Tailwind conflicts (shadcn convention).
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
