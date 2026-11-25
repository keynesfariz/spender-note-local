import { XIcon } from "lucide-react"
import type { ComponentProps, ReactNode } from "react"

import { IconTooltip } from "@/components/icon-tooltip"
import { Button } from "@/components/ui/button"
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer"

type DetailDrawerContentProps = {
  title: string
  description: string
  children: ReactNode
  footer?: ReactNode
} & Pick<ComponentProps<typeof DrawerContent>, "initialFocus" | "finalFocus">

export function DetailDrawerContent({
  title,
  description,
  children,
  footer,
  initialFocus,
  finalFocus,
}: DetailDrawerContentProps) {
  return (
    <DrawerContent
      className="h-dvh w-full max-w-2xl shadow-2xl data-[swipe-direction=right]:rounded-none data-[swipe-direction=right]:[--drawer-inset:0px] motion-reduce:transition-none"
      initialFocus={initialFocus}
      finalFocus={finalFocus}
    >
      <DrawerHeader className="border-b bg-muted/50 px-5 py-5 pr-14 sm:px-6 sm:pr-14">
        <DrawerTitle className="text-lg font-semibold tracking-tight">
          {title}
        </DrawerTitle>
        <DrawerDescription>{description}</DrawerDescription>
      </DrawerHeader>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-6 sm:px-6">
        {children}
      </div>
      {footer}
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

export function CreateFormDrawer({
  open,
  onOpenChange,
  formId,
  submitLabel,
  submitDisabled,
  ...content
}: Omit<DetailDrawerContentProps, "footer"> & {
  open: boolean
  onOpenChange: (open: boolean) => void
  formId: string
  submitLabel: string
  submitDisabled?: boolean
}) {
  return (
    <Drawer open={open} onOpenChange={onOpenChange} swipeDirection="right">
      <DetailDrawerContent
        {...content}
        footer={
          <DrawerFooter className="flex-row flex-wrap justify-end border-t px-5 py-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:px-6">
            <DrawerClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DrawerClose>
            <Button type="submit" form={formId} disabled={submitDisabled}>
              {submitLabel}
            </Button>
          </DrawerFooter>
        }
      />
    </Drawer>
  )
}
