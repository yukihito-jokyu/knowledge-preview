import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";

import { cn } from "../../lib/utils";

const Empty = React.forwardRef<HTMLDivElement, React.ComponentPropsWithoutRef<"div">>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="empty"
      className={cn(
        "flex min-w-0 flex-1 flex-col items-center justify-center gap-6 text-balance rounded-2xl border border-border/80 bg-card/85 p-6 text-center text-card-foreground shadow-soft backdrop-blur-xl md:p-10",
        className,
      )}
      {...props}
    />
  ),
);

Empty.displayName = "Empty";

const EmptyHeader = React.forwardRef<HTMLDivElement, React.ComponentPropsWithoutRef<"div">>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="empty-header"
      className={cn("flex max-w-sm flex-col items-center gap-2 text-center", className)}
      {...props}
    />
  ),
);

EmptyHeader.displayName = "EmptyHeader";

const emptyMediaVariants = cva(
  "mb-2 flex shrink-0 items-center justify-center [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        icon: "bg-accent text-accent-foreground flex size-11 shrink-0 items-center justify-center rounded-2xl shadow-soft [&_svg:not([class*='size-'])]:size-6",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

const EmptyMedia = React.forwardRef<
  HTMLDivElement,
  React.ComponentPropsWithoutRef<"div"> & VariantProps<typeof emptyMediaVariants>
>(({ className, variant = "default", ...props }, ref) => (
  <div
    ref={ref}
    data-slot="empty-icon"
    data-variant={variant}
    className={cn(emptyMediaVariants({ variant, className }))}
    {...props}
  />
));

EmptyMedia.displayName = "EmptyMedia";

const EmptyTitle = React.forwardRef<HTMLDivElement, React.ComponentPropsWithoutRef<"div">>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="empty-title"
      className={cn("text-lg font-medium tracking-tight", className)}
      {...props}
    />
  ),
);

EmptyTitle.displayName = "EmptyTitle";

const EmptyDescription = React.forwardRef<
  HTMLParagraphElement,
  React.ComponentPropsWithoutRef<"p">
>(({ className, ...props }, ref) => (
  <p
    ref={ref}
    data-slot="empty-description"
    className={cn(
      "text-muted-foreground [&>a:hover]:text-primary text-base/relaxed [&>a]:underline [&>a]:underline-offset-4",
      className,
    )}
    {...props}
  />
));

EmptyDescription.displayName = "EmptyDescription";

const EmptyContent = React.forwardRef<HTMLDivElement, React.ComponentPropsWithoutRef<"div">>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="empty-content"
      className={cn(
        "flex w-full min-w-0 max-w-sm flex-col items-center gap-4 text-balance text-base",
        className,
      )}
      {...props}
    />
  ),
);

EmptyContent.displayName = "EmptyContent";

export { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle };
