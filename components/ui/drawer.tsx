"use client"

import { IconTooltip } from "@/components/ui/tooltip"
import { Drawer as DrawerPrimitive } from "@base-ui/react/drawer"
import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

function Drawer(props: DrawerPrimitive.Root.Props) {
  return <DrawerPrimitive.Root swipeDirection="right" {...props} />
}

function DrawerContent({ children }: { children: React.ReactNode }) {
  return (
    <DrawerPrimitive.Portal>
      <DrawerPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/50 transition-opacity data-ending-style:opacity-0 data-starting-style:opacity-0 motion-reduce:transition-none" />
      <DrawerPrimitive.Viewport className="pointer-events-none fixed inset-0 z-50 flex justify-end">
        <DrawerPrimitive.Popup className="pointer-events-auto relative flex h-full w-full max-w-2xl translate-x-(--drawer-swipe-movement-x) flex-col border-l bg-popover text-popover-foreground shadow-2xl transition-transform duration-200 outline-none data-ending-style:translate-x-full data-starting-style:translate-x-full data-swiping:transition-none motion-reduce:transition-none">
          {children}
          <IconTooltip label="Close">
            <DrawerPrimitive.Close
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="absolute top-4 right-4"
                />
              }
              aria-label="Close drawer"
            >
              <XIcon />
            </DrawerPrimitive.Close>
          </IconTooltip>
        </DrawerPrimitive.Popup>
      </DrawerPrimitive.Viewport>
    </DrawerPrimitive.Portal>
  )
}

function DrawerHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="space-y-1 border-b bg-muted/50 px-5 py-5 pr-14 sm:px-6 sm:pr-14">
      {children}
    </div>
  )
}

function DrawerTitle(props: DrawerPrimitive.Title.Props) {
  return (
    <DrawerPrimitive.Title
      className="text-lg font-semibold tracking-tight"
      {...props}
    />
  )
}

function DrawerDescription(props: DrawerPrimitive.Description.Props) {
  return (
    <DrawerPrimitive.Description
      className="text-sm text-muted-foreground"
      {...props}
    />
  )
}

function DrawerBody({ children }: { children: React.ReactNode }) {
  return (
    <DrawerPrimitive.Content className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-6 sm:px-6">
      {children}
    </DrawerPrimitive.Content>
  )
}

const DrawerTrigger = DrawerPrimitive.Trigger

export {
  Drawer,
  DrawerTrigger,
  DrawerBody,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
}
