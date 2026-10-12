import { ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { Popover, Tooltip } from 'antd';
import clsx from 'clsx';
import { useAtomValue, useSetAtom } from 'jotai';
import { useMediaQuery } from 'react-responsive';

import { isMacroPlayingAtom } from '@/jotai/mouse.ts';
import { submenuOpenCountAtom } from '@/jotai/settings.ts';

type MenuItemProps = {
  title: string;
  icon: ReactNode;
  content: ReactNode;
  className?: string;
  fresh?: boolean;
  onOpenChange?: (open: boolean) => void;
};

export const MenuItem = ({
  title,
  icon,
  content,
  className,
  fresh,
  onOpenChange
}: MenuItemProps) => {
  const isBigScreen = useMediaQuery({ minWidth: 640 });
  const setSubmenuOpenCount = useSetAtom(submenuOpenCountAtom);
  const isMacroPlaying = useAtomValue(isMacroPlayingAtom);

  const [isPopoverOpen, setIsPopoverOpen] = useState(false);
  const [isTooltipOpen, setIsTooltipOpen] = useState(false);
  const isPopoverOpenRef = useRef(false);
  const isMacroPlayingRef = useRef(isMacroPlaying);
  isMacroPlayingRef.current = isMacroPlaying;

  const togglePopover = useCallback(
    (open: boolean) => {
      if (open && isMacroPlayingRef.current) {
        return;
      }
      if (isPopoverOpenRef.current === open) {
        return;
      }

      isPopoverOpenRef.current = open;
      setIsTooltipOpen(false);
      setIsPopoverOpen(open);
      setSubmenuOpenCount((count) => (open ? count + 1 : Math.max(0, count - 1)));
      onOpenChange?.(open);
    },
    [onOpenChange, setSubmenuOpenCount]
  );

  useEffect(() => {
    if (isMacroPlaying) {
      togglePopover(false);
    }
  }, [isMacroPlaying, togglePopover]);

  function toggleTooltip(open: boolean) {
    if (isPopoverOpen) {
      return;
    }
    setIsTooltipOpen(open);
  }

  return (
    <Popover
      content={content}
      arrow={false}
      trigger="click"
      zIndex={900}
      placement={isBigScreen ? 'bottomLeft' : 'bottom'}
      open={isPopoverOpen}
      onOpenChange={togglePopover}
      fresh={!!fresh}
    >
      <Tooltip
        title={title}
        mouseEnterDelay={0.6}
        placement="bottom"
        open={isTooltipOpen}
        onOpenChange={toggleTooltip}
      >
        <div
          className={clsx(
            'flex h-[30px] w-[30px] cursor-pointer items-center justify-center rounded text-neutral-300 hover:bg-neutral-700/80 hover:text-white',
            className
          )}
        >
          {icon}
        </div>
      </Tooltip>
    </Popover>
  );
};
