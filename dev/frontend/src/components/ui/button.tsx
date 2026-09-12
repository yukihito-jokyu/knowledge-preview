import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";

import { cn } from "../../lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-xl text-base font-semibold transition-all duration-200 ease-out focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-ring/20 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "bg-linear-to-r from-primary to-primary-end text-primary-foreground shadow-[0_10px_24px_hsl(var(--primary)/0.28)] hover:-translate-y-0.5 hover:shadow-[0_15px_30px_hsl(var(--primary)/0.34)] active:translate-y-0",
        destructive:
          "bg-destructive text-destructive-foreground shadow-soft hover:-translate-y-px hover:bg-destructive/90 active:translate-y-0",
        outline:
          "border border-border/80 bg-card/80 shadow-soft backdrop-blur-sm hover:-translate-y-px hover:bg-accent hover:text-accent-foreground hover:shadow-raised active:translate-y-0",
        secondary:
          "bg-secondary/90 text-secondary-foreground shadow-soft hover:-translate-y-px hover:bg-secondary hover:shadow-raised active:translate-y-0",
        ghost: "hover:bg-accent/85 hover:text-accent-foreground focus-visible:bg-accent",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-9 rounded-lg px-3 text-sm",
        lg: "h-11 rounded-xl px-8",
        icon: "size-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

type ButtonProps = React.ComponentPropsWithoutRef<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean;
  };

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        data-slot="button"
        className={cn(buttonVariants({ variant, size }), className)}
        ref={ref}
        {...props}
      />
    );
  },
);

Button.displayName = "Button";

export { Button, buttonVariants };
