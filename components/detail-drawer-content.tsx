import { XIcon } from "lucide-react"
import type { ReactNode } from "react"

import { IconTooltip } from "@/components/icon-tooltip"
import { Button } from "@/components/ui/button"
import {
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer"

export function DetailDrawerContent({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <DrawerContent className="h-dvh w-full max-w-2xl shadow-2xl data-[swipe-direction=right]:rounded-none motion-reduce:transition-none">
      <DrawerHeader className="border-b bg-muted/50 px-5 py-5 pr-14 sm:px-6 sm:pr-14">
        <DrawerTitle className="text-lg font-semibold tracking-tight">
          {title}
        </DrawerTitle>
        <DrawerDescription>{description}</DrawerDescription>
      </DrawerHeader>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-6 sm:px-6">
        {children}
      </div>
      <IconTooltip label="Close">
        <DrawerClose
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              className="absolute top-4 right-4"
            />
          }
          aria-label="Close drawer"
        >
          <XIcon aria-hidden="true" />
        </DrawerClose>
      </IconTooltip>
    </DrawerContent>
  )
}
