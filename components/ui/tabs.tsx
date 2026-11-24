"use client"

import { Tabs as TabsPrimitive } from "@base-ui/react/tabs"

import { cn } from "@/lib/utils"

function Tabs({ className, ...props }: TabsPrimitive.Root.Props) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      className={cn("flex flex-col gap-4", className)}
      {...props}
    />
  )
}

function TabsList({
  className,
  variant = "default",
  ...props
}: TabsPrimitive.List.Props & {
  variant?: "default" | "line"
}) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      data-variant={variant}
      className={cn(
        "group/tabs-list inline-flex items-center gap-1 overflow-x-auto text-muted-foreground",
        variant === "line"
          ? "border-b"
          : "w-fit max-w-full rounded-lg bg-muted p-1",
        className
      )}
      {...props}
    />
  )
}

function TabsTab({ className, ...props }: TabsPrimitive.Tab.Props) {
  return (
    <TabsPrimitive.Tab
      data-slot="tabs-tab"
      className={cn(
        "inline-flex shrink-0 items-center justify-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-all outline-none group-data-[variant=line]/tabs-list:rounded-none group-data-[variant=line]/tabs-list:border-b-2 group-data-[variant=line]/tabs-list:border-transparent group-data-[variant=line]/tabs-list:py-2.5 group-data-[variant=line]/tabs-list:hover:text-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-selected:bg-background aria-selected:text-foreground aria-selected:shadow-xs group-data-[variant=line]/tabs-list:aria-selected:border-primary group-data-[variant=line]/tabs-list:aria-selected:bg-transparent group-data-[variant=line]/tabs-list:aria-selected:text-primary group-data-[variant=line]/tabs-list:aria-selected:shadow-none",
        className
      )}
      {...props}
    />
  )
}

function TabsPanel({ className, ...props }: TabsPrimitive.Panel.Props) {
  return (
    <TabsPrimitive.Panel
      data-slot="tabs-panel"
      className={cn("space-y-6 outline-none", className)}
      {...props}
    />
  )
}

export { Tabs, TabsList, TabsPanel, TabsTab }
