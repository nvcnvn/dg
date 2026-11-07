shape "nine" {
  svg = file("hostile.svg")
}

shape "fit" {
  svg     = file("hostile.svg")
  stretch = "uniform"
}

shape "natural" {
  svg     = file("hostile.svg")
  stretch = "none"
}

style "badge" {
  icon = file("hostile.svg")
}

style "photo" {
  shape = "image"
  image = file("hostile.svg")
}

style "nine" {
  shape = "nine"
}

style "fit" {
  shape = "fit"
}

style "natural" {
  shape = "natural"
}
