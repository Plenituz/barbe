# To run this:
# Invoke-WebRequest -UseBasicParsing -Uri https://hub.barbe.app/install.ps1 | Invoke-Expression

$base="https://github.com/Plenituz/barbe/releases"
$fileName="windows_amd64.zip"
function http_download {
    $version=Get_Version
    $url = $base + "/download/" + $version + "/" + $fileName
    write-host $url

    Invoke-WebRequest -Uri $url -OutFile $env:temp/$fileName -ErrorAction Stop
    Expand-Archive -Path $env:temp/$fileName -DestinationPath $env:HOMEPATH/barbe -Force -ErrorVariable ProcessError;
    If ($ProcessError)
    {
        write-host "there was an error unzipping the file, you can try again or unzip it manually: " $env:temp/$fileName
        exit
    }
    write-host "Barbe has been installed at $env:HOMEPATH\barbe, add it to your PATH to use it (https://stackoverflow.com/questions/1618280/where-can-i-set-path-to-make-exe-on-windows). Make sure you also have Docker installed and running."
}

function Get_Version {
    $versionUrl = $base + "/latest"
    $headers = @{
        'Accept' = 'application/json'
    }
    $response = Invoke-RestMethod $versionUrl -Method 'GET' -Headers $headers -ErrorAction SilentlyContinue -ErrorVariable DownloadError
    If ($DownloadError)
    {
        write-host "Error downloading from github, please check your internet connection"
        exit
    }
    return $response.tag_name
}

http_download